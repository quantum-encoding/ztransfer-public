package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/quantum-encoding/ztransfer/pkg/server"
)

// uploadFile uploads a local file to the selected peer, updating the provided UI elements.
func (c *Controller) uploadFile(
	localPath, remotePath string,
	progressBar *widget.ProgressBar,
	progressLabel *widget.Label,
	cancelButton *widget.Button,
	fileStatus *widget.Label,
	transferCancel *func(),
	refreshFiles func(),
) {
	progressBar.Show()
	progressBar.SetValue(0)
	progressLabel.Show()
	cancelButton.Show()
	progressLabel.SetText(fmt.Sprintf("Uploading %s...", filepath.Base(localPath)))
	c.SetStatus(fmt.Sprintf("Uploading %s", filepath.Base(localPath)))

	ctx, cancel := context.WithCancel(context.Background())
	*transferCancel = cancel

	go func() {
		written, err := c.client.Upload(c.selectedPeer, localPath, remotePath)
		_ = ctx
		fyne.Do(func() {
			cancelButton.Hide()
			progressBar.SetValue(1.0)
			if err != nil {
				fileStatus.SetText("Upload error: " + err.Error())
				c.SetStatus("Upload failed")
			} else {
				fileStatus.SetText(fmt.Sprintf("Uploaded %s (%s)", filepath.Base(localPath), formatBytes(written)))
				c.SetStatus("Upload complete")
				if refreshFiles != nil {
					refreshFiles()
				}
			}
		})
	}()
}

// BuildTransferTab creates the main file transfer interface.
func (c *Controller) BuildTransferTab(w fyne.Window) fyne.CanvasObject {
	// === Peer list (left panel) ===
	peerList := widget.NewList(
		func() int {
			return len(c.peerStore.ListPeers())
		},
		func() fyne.CanvasObject {
			return container.NewVBox(
				widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				widget.NewLabel(""),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			peers := c.peerStore.ListPeers()
			if id >= len(peers) {
				return
			}
			p := peers[id]
			box := obj.(*fyne.Container)
			box.Objects[0].(*widget.Label).SetText(p.Name)
			box.Objects[1].(*widget.Label).SetText(p.Address)
		},
	)

	// === File list (right panel) ===
	var files []server.FileInfo
	var filesMu sync.Mutex
	currentPath := "/"

	pathLabel := widget.NewLabelWithStyle("/", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})

	var fileList *widget.List
	fileList = widget.NewList(
		func() int {
			filesMu.Lock()
			defer filesMu.Unlock()
			return len(files)
		},
		func() fyne.CanvasObject {
			inner := container.NewHBox(
				widget.NewIcon(theme.FileIcon()),
				widget.NewLabel("filename.txt"),
				layout.NewSpacer(),
				widget.NewLabel("0 B"),
			)
			return newRightClickable(inner, nil)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			filesMu.Lock()
			if id >= len(files) {
				filesMu.Unlock()
				return
			}
			f := files[id]
			filesMu.Unlock()

			rc := obj.(*rightClickable)
			box := rc.child.(*fyne.Container)
			icon := box.Objects[0].(*widget.Icon)
			nameLabel := box.Objects[1].(*widget.Label)
			sizeLabel := box.Objects[3].(*widget.Label)

			if f.IsDir {
				icon.SetResource(theme.FolderIcon())
				nameLabel.SetText(f.Name + "/")
				sizeLabel.SetText("")
			} else {
				icon.SetResource(theme.FileIcon())
				nameLabel.SetText(f.Name)
				sizeLabel.SetText(formatBytes(f.Size))
			}

			rc.onRightTap = func(_ fyne.Position, abs fyne.Position) {
				items := []*fyne.MenuItem{
					fyne.NewMenuItem("Copy Name", func() {
						w.Clipboard().SetContent(f.Name)
					}),
					fyne.NewMenuItem("Copy Path", func() {
						w.Clipboard().SetContent(f.Path)
					}),
				}
				if !f.IsDir {
					items = append(items, fyne.NewMenuItem("Download", func() {
						fileList.Select(id)
					}))
				}
				menu := fyne.NewMenu("", items...)
				widget.ShowPopUpMenuAtPosition(menu, w.Canvas(), abs)
			}
		},
	)

	// Progress bar + cancel
	progressBar := widget.NewProgressBar()
	progressBar.Hide()
	progressLabel := widget.NewLabel("")
	progressLabel.Hide()

	var transferCancel func()
	cancelButton := widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
		if transferCancel != nil {
			transferCancel()
			transferCancel = nil
		}
		progressBar.Hide()
		progressLabel.Hide()
	})
	cancelButton.Hide()
	cancelButton.Importance = widget.DangerImportance

	// Status label for file operations
	fileStatus := widget.NewLabel("")

	// Refresh file list from remote
	refreshFiles := func() {
		if c.selectedPeer == "" {
			return
		}
		c.SetStatus(fmt.Sprintf("Loading %s:%s...", c.selectedPeer, currentPath))
		go func() {
			result, err := c.client.List(c.selectedPeer, currentPath)
			filesMu.Lock()
			if err == nil {
				files = result
			}
			filesMu.Unlock()
			fyne.Do(func() {
				if err != nil {
					fileStatus.SetText("Error: " + err.Error())
					c.SetStatus("Error listing files")
				} else {
					pathLabel.SetText(currentPath)
					fileList.Refresh()
					c.SetStatus(fmt.Sprintf("Connected to %s", c.selectedPeer))
				}
			})
		}()
	}

	// Peer selection -> load files
	peerList.OnSelected = func(id widget.ListItemID) {
		peers := c.peerStore.ListPeers()
		if id < len(peers) {
			c.selectedPeer = peers[id].Name
			currentPath = "/"
			refreshFiles()
		}
	}

	// Track selected file index
	selectedFileIdx := -1

	// Double-click file to navigate into dirs or download files
	fileList.OnSelected = func(id widget.ListItemID) {
		filesMu.Lock()
		if id >= len(files) {
			filesMu.Unlock()
			return
		}
		f := files[id]
		filesMu.Unlock()

		selectedFileIdx = id

		if f.IsDir {
			currentPath = f.Path + "/"
			selectedFileIdx = -1
			refreshFiles()
		}
	}

	// === Action buttons ===
	upButton := widget.NewButtonWithIcon("Up", theme.NavigateBackIcon(), func() {
		if currentPath != "/" {
			currentPath = filepath.Dir(filepath.Clean(currentPath))
			if currentPath != "/" {
				currentPath += "/"
			}
			refreshFiles()
		}
	})

	refreshButton := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		refreshFiles()
	})

	downloadButton := widget.NewButtonWithIcon("Download", theme.DownloadIcon(), func() {
		filesMu.Lock()
		if selectedFileIdx < 0 || selectedFileIdx >= len(files) {
			filesMu.Unlock()
			return
		}
		f := files[selectedFileIdx]
		filesMu.Unlock()

		if f.IsDir {
			fileStatus.SetText("Cannot download a directory")
			return
		}

		progressBar.Show()
		progressBar.SetValue(0)
		progressLabel.Show()
		cancelButton.Show()
		progressLabel.SetText(fmt.Sprintf("Downloading %s...", f.Name))
		c.SetStatus(fmt.Sprintf("Downloading %s", f.Name))

		ctx, cancel := context.WithCancel(context.Background())
		transferCancel = cancel

		go func() {
			written, err := c.client.Download(c.selectedPeer, f.Path, c.downloadDir)
			_ = ctx // cancel support for future streaming downloads
			fyne.Do(func() {
				cancelButton.Hide()
				progressBar.SetValue(1.0)
				if err != nil {
					fileStatus.SetText("Download error: " + err.Error())
					progressLabel.SetText("Download failed")
					c.SetStatus("Download failed")
				} else {
					fileStatus.SetText(fmt.Sprintf("Downloaded %s (%s)", f.Name, formatBytes(written)))
					progressLabel.SetText(fmt.Sprintf("Downloaded %s to %s", f.Name, c.downloadDir))
					c.SetStatus("Download complete")
				}
			})
		}()
	})

	uploadButton := widget.NewButtonWithIcon("Upload", theme.UploadIcon(), func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			reader.Close()
			localPath := reader.URI().Path()
			remotePath := currentPath + filepath.Base(localPath)
			c.uploadFile(localPath, remotePath, progressBar, progressLabel, cancelButton, fileStatus, &transferCancel, refreshFiles)
		}, w)
		fd.Resize(fyne.NewSize(800, 560))
		fd.Show()
	})

	// Drop zone label
	dropHint := widget.NewLabelWithStyle("Drop files here to upload", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	// Register drop handler on the controller so main.go can wire SetOnDropped
	c.mu.Lock()
	c.onDrop = func(uris []fyne.URI) {
		if c.selectedPeer == "" {
			fileStatus.SetText("Select a peer before dropping files")
			return
		}
		for _, uri := range uris {
			localPath := uri.Path()
			remotePath := currentPath + filepath.Base(localPath)
			c.uploadFile(localPath, remotePath, progressBar, progressLabel, cancelButton, fileStatus, &transferCancel, refreshFiles)
		}
	}
	c.mu.Unlock()

	// Download dir selector
	downloadDirLabel := widget.NewLabelWithStyle(c.downloadDir, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	browseDownloadDir := widget.NewButtonWithIcon("Browse", theme.FolderOpenIcon(), func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			c.downloadDir = uri.Path()
			downloadDirLabel.SetText(c.downloadDir)
		}, w)
		fd.Resize(fyne.NewSize(800, 560))
		fd.Show()
	})

	// === Layout ===
	navBar := container.NewHBox(
		upButton,
		refreshButton,
		layout.NewSpacer(),
		pathLabel,
		layout.NewSpacer(),
	)

	actionBar := container.NewHBox(
		widget.NewLabel("Download to:"),
		downloadDirLabel,
		browseDownloadDir,
		layout.NewSpacer(),
		downloadButton,
		uploadButton,
	)

	progressSection := container.NewVBox(
		container.NewHBox(progressBar, cancelButton),
		progressLabel,
	)

	rightPanel := container.NewBorder(
		navBar,
		container.NewVBox(actionBar, progressSection, fileStatus, dropHint),
		nil, nil,
		fileList,
	)

	peerPanel := panelWithTitle("Peers", peerList)

	split := container.NewHSplit(peerPanel, rightPanel)
	split.SetOffset(0.22)

	return split
}

