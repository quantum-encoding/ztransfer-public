package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// maxPreviewSize is the maximum file size we'll load into the viewer.
const maxPreviewSize = 10 * 1024 * 1024 // 10 MB

// dirCache caches directory listings for the tree so we don't re-read on every render.
type dirCache struct {
	mu    sync.Mutex
	cache map[string][]os.DirEntry
}

func newDirCache() *dirCache {
	return &dirCache{cache: make(map[string][]os.DirEntry)}
}

func (dc *dirCache) Get(path string) ([]os.DirEntry, error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if cached, ok := dc.cache[path]; ok {
		return cached, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	// Sort: directories first, then alphabetical
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	dc.cache[path] = entries
	return entries, nil
}

func (dc *dirCache) Invalidate(path string) {
	dc.mu.Lock()
	delete(dc.cache, path)
	dc.mu.Unlock()
}

func (dc *dirCache) InvalidateAll() {
	dc.mu.Lock()
	dc.cache = make(map[string][]os.DirEntry)
	dc.mu.Unlock()
}

// BuildFilesTab creates the local file system browser with split view:
// left panel = directory tree, right panel = file viewer.
func (c *Controller) BuildFilesTab(w fyne.Window) fyne.CanvasObject {
	home, _ := os.UserHomeDir()
	rootDir := home
	dc := newDirCache()

	// === Path bar ===
	pathLabel := widget.NewLabelWithStyle(rootDir, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})

	// === File viewer (right panel) ===
	viewerContent := widget.NewRichTextFromMarkdown("*Select a file to view*")
	viewerContent.Wrapping = fyne.TextWrapWord
	viewerScroll := container.NewScroll(viewerContent)

	viewerFileName := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	viewerFileInfo := widget.NewLabel("")

	viewerHeader := container.NewVBox(
		viewerFileName,
		viewerFileInfo,
		widget.NewSeparator(),
	)

	// Track currently viewed file for context actions
	var selectedFilePath string

	// File action buttons (below viewer)
	copyPathButton := widget.NewButtonWithIcon("Copy Path", theme.ContentCopyIcon(), func() {
		if selectedFilePath != "" {
			w.Clipboard().SetContent(selectedFilePath)
			viewerFileInfo.SetText("Path copied to clipboard")
		}
	})

	openFinderButton := widget.NewButtonWithIcon("Reveal", theme.FolderOpenIcon(), func() {
		if selectedFilePath != "" {
			openInFileManager(selectedFilePath)
		}
	})

	viewerActions := container.NewHBox(
		copyPathButton,
		openFinderButton,
		layout.NewSpacer(),
	)

	viewerPanel := container.NewBorder(
		viewerHeader,
		viewerActions,
		nil, nil,
		viewerScroll,
	)

	// Load file into viewer
	loadFile := func(path string) {
		info, err := os.Stat(path)
		if err != nil {
			viewerContent.ParseMarkdown(fmt.Sprintf("**Error:** %v", err))
			return
		}

		viewerFileName.SetText(filepath.Base(path))
		viewerFileInfo.SetText(fmt.Sprintf("%s  |  %s  |  %s",
			formatBytes(info.Size()),
			info.ModTime().Format("2006-01-02 15:04"),
			info.Mode().String(),
		))

		if info.Size() > maxPreviewSize {
			viewerContent.ParseMarkdown(fmt.Sprintf("*File too large to preview (%s)*", formatBytes(info.Size())))
			return
		}

		ext := strings.ToLower(filepath.Ext(path))

		// Binary files — show info only
		if isBinaryExt(ext) {
			viewerContent.ParseMarkdown(fmt.Sprintf("**Binary file:** %s\n\n*%s, %s*",
				filepath.Base(path), ext, formatBytes(info.Size())))
			return
		}

		// Text files — read and display
		f, err := os.Open(path)
		if err != nil {
			viewerContent.ParseMarkdown(fmt.Sprintf("**Error:** %v", err))
			return
		}
		defer f.Close()

		data, err := io.ReadAll(io.LimitReader(f, maxPreviewSize))
		if err != nil {
			viewerContent.ParseMarkdown(fmt.Sprintf("**Error reading:** %v", err))
			return
		}

		content := string(data)

		// Markdown files render as markdown
		if ext == ".md" || ext == ".markdown" {
			viewerContent.ParseMarkdown(content)
		} else {
			// Code/text files — wrap in code block
			lang := langForExt(ext)
			if lang != "" {
				viewerContent.ParseMarkdown(fmt.Sprintf("```%s\n%s\n```", lang, content))
			} else {
				viewerContent.ParseMarkdown(fmt.Sprintf("```\n%s\n```", content))
			}
		}
	}

	// === Directory tree (left panel) ===
	// Tree node IDs are absolute file paths. Root "" maps to rootDir.
	var fileTree *widget.Tree
	fileTree = widget.NewTree(
		// childUIDs: return children of a node
		func(uid widget.TreeNodeID) []widget.TreeNodeID {
			dir := uid
			if dir == "" {
				dir = rootDir
			}
			entries, err := dc.Get(dir)
			if err != nil {
				return nil
			}
			children := make([]widget.TreeNodeID, len(entries))
			for i, e := range entries {
				children[i] = filepath.Join(dir, e.Name())
			}
			return children
		},
		// isBranch: directories are branches
		func(uid widget.TreeNodeID) bool {
			if uid == "" {
				return true
			}
			info, err := os.Stat(uid)
			if err != nil {
				return false
			}
			return info.IsDir()
		},
		// createNode: layout for each tree row
		func(branch bool) fyne.CanvasObject {
			inner := container.NewHBox(
				widget.NewIcon(theme.FileIcon()),
				widget.NewLabel("filename.txt"),
				layout.NewSpacer(),
				widget.NewLabel(""),
			)
			return newRightClickable(inner, nil)
		},
		// updateNode: populate each row
		func(uid widget.TreeNodeID, branch bool, obj fyne.CanvasObject) {
			rc := obj.(*rightClickable)
			box := rc.child.(*fyne.Container)
			icon := box.Objects[0].(*widget.Icon)
			nameLabel := box.Objects[1].(*widget.Label)
			sizeLabel := box.Objects[3].(*widget.Label)

			name := filepath.Base(uid)

			if branch {
				icon.SetResource(theme.FolderIcon())
				nameLabel.SetText(name)
				sizeLabel.SetText("")
			} else {
				icon.SetResource(fileIconForName(name))
				nameLabel.SetText(name)
				info, err := os.Stat(uid)
				if err == nil {
					sizeLabel.SetText(formatBytes(info.Size()))
				} else {
					sizeLabel.SetText("?")
				}
			}

			rc.onRightTap = func(_ fyne.Position, abs fyne.Position) {
				items := []*fyne.MenuItem{
					fyne.NewMenuItem("Copy Path", func() {
						w.Clipboard().SetContent(uid)
						if !branch {
							viewerFileInfo.SetText("Path copied to clipboard")
						}
					}),
					fyne.NewMenuItem("Reveal in File Manager", func() {
						openInFileManager(uid)
					}),
				}
				if !branch {
					items = append(items, fyne.NewMenuItem("View", func() {
						selectedFilePath = uid
						loadFile(uid)
					}))
				} else {
					items = append(items, fyne.NewMenuItem("Refresh", func() {
						dc.Invalidate(uid)
						fileTree.Refresh()
					}))
				}
				menu := fyne.NewMenu("", items...)
				widget.ShowPopUpMenuAtPosition(menu, w.Canvas(), abs)
			}
		},
	)

	// Select a file -> preview it; select a dir -> update path label
	fileTree.OnSelected = func(uid widget.TreeNodeID) {
		info, err := os.Stat(uid)
		if err != nil {
			return
		}
		pathLabel.SetText(uid)
		if !info.IsDir() {
			selectedFilePath = uid
			loadFile(uid)
		}
	}

	// === Navigation buttons ===
	setRoot := func(dir string) {
		rootDir = dir
		dc.InvalidateAll()
		pathLabel.SetText(dir)
		fileTree.Refresh()
		fileTree.UnselectAll()
	}

	upButton := widget.NewButtonWithIcon("Up", theme.NavigateBackIcon(), func() {
		parent := filepath.Dir(rootDir)
		if parent != rootDir {
			setRoot(parent)
		}
	})

	homeButton := widget.NewButtonWithIcon("Home", theme.HomeIcon(), func() {
		setRoot(home)
	})

	refreshButton := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		dc.InvalidateAll()
		fileTree.Refresh()
	})

	browseButton := widget.NewButtonWithIcon("Open", theme.FolderOpenIcon(), func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			setRoot(uri.Path())
		}, w)
		fd.Resize(fyne.NewSize(800, 560))
		fd.Show()
	})

	// === Whitelist section ===
	var whitelistList *widget.List
	whitelistList = widget.NewList(
		func() int {
			c.mu.RLock()
			defer c.mu.RUnlock()
			return len(c.whitelistedDirs)
		},
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.FolderIcon()),
				widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true}),
				layout.NewSpacer(),
				widget.NewButtonWithIcon("", theme.DeleteIcon(), nil),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			c.mu.RLock()
			if id >= len(c.whitelistedDirs) {
				c.mu.RUnlock()
				return
			}
			dir := c.whitelistedDirs[id]
			c.mu.RUnlock()

			box := obj.(*fyne.Container)
			box.Objects[1].(*widget.Label).SetText(dir)
			box.Objects[3].(*widget.Button).OnTapped = func() {
				c.RemoveWhitelistedDir(dir)
				whitelistList.Refresh()
			}
		},
	)

	addWhitelistButton := widget.NewButtonWithIcon("Add Directory", theme.ContentAddIcon(), func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			c.AddWhitelistedDir(uri.Path())
			whitelistList.Refresh()
		}, w)
		fd.Resize(fyne.NewSize(800, 560))
		fd.Show()
	})

	addCurrentButton := widget.NewButtonWithIcon("Add Current", theme.FolderIcon(), func() {
		c.AddWhitelistedDir(rootDir)
		whitelistList.Refresh()
	})

	// === Layout ===
	navBar := container.NewHBox(
		upButton,
		homeButton,
		refreshButton,
		browseButton,
		layout.NewSpacer(),
		pathLabel,
		layout.NewSpacer(),
	)

	whitelistControls := container.NewHBox(
		addWhitelistButton,
		addCurrentButton,
	)

	whitelistSection := panelWithTitle("Discoverable Directories", container.NewBorder(
		whitelistControls,
		nil, nil, nil,
		whitelistList,
	))

	// Tree panel (top) + whitelist (bottom)
	leftContent := container.NewBorder(
		navBar,
		nil, nil, nil,
		container.NewVSplit(
			fileTree,
			whitelistSection,
		),
	)

	// Main split: tree left, viewer right
	split := container.NewHSplit(leftContent, viewerPanel)
	split.SetOffset(0.35)

	return split
}

// fileIconForName returns an appropriate icon resource based on file extension.
func fileIconForName(name string) fyne.Resource {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go", ".rs", ".py", ".js", ".ts", ".c", ".h", ".cpp", ".java",
		".rb", ".php", ".swift", ".kt", ".zig", ".lua":
		return theme.FileApplicationIcon()
	case ".md", ".txt", ".log", ".csv", ".json", ".yaml", ".yml",
		".toml", ".xml", ".html", ".css", ".sql", ".sh", ".bash":
		return theme.FileTextIcon()
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".bmp", ".ico":
		return theme.FileImageIcon()
	case ".mp4", ".mkv", ".avi", ".mov", ".webm":
		return theme.FileVideoIcon()
	default:
		return theme.FileIcon()
	}
}

// isBinaryExt returns true for file extensions that are binary (not text-previewable).
func isBinaryExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".webp",
		".mp4", ".mkv", ".avi", ".mov", ".webm", ".mp3", ".flac", ".wav", ".ogg",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".zst", ".7z", ".rar",
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".exe", ".dll", ".so", ".dylib", ".a", ".o", ".class",
		".wasm", ".bin", ".dat", ".db", ".sqlite":
		return true
	}
	return false
}

// langForExt returns the markdown code fence language for syntax highlighting.
func langForExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".py":
		return "python"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".mts":
		return "typescript"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".java":
		return "java"
	case ".sh", ".bash", ".zsh":
		return "bash"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".sql":
		return "sql"
	case ".zig":
		return "zig"
	case ".lua":
		return "lua"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".svelte":
		return "svelte"
	case ".dockerfile":
		return "dockerfile"
	}
	return ""
}
