package main

import (
	"fmt"
	"path/filepath"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/quantum-encoding/ztransfer-public/pkg/server"
)

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

	fileList := widget.NewList(
		func() int {
			filesMu.Lock()
			defer filesMu.Unlock()
			return len(files)
		},
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.FileIcon()),
				widget.NewLabel("filename.txt"),
				layout.NewSpacer(),
				widget.NewLabel("0 B"),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			filesMu.Lock()
			if id >= len(files) {
				filesMu.Unlock()
				return
			}
			f := files[id]
			filesMu.Unlock()

			box := obj.(*fyne.Container)
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
		},
	)

	// Progress bar
	progressBar := widget.NewProgressBar()
	progressBar.Hide()
	progressLabel := widget.NewLabel("")
	progressLabel.Hide()

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
			if err != nil {
				fileStatus.SetText("Error: " + err.Error())
				c.SetStatus("Error listing files")
				return
			}
			filesMu.Lock()
			files = result
			filesMu.Unlock()
			pathLabel.SetText(currentPath)
			fileList.Refresh()
			c.SetStatus(fmt.Sprintf("Connected to %s", c.selectedPeer))
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
		progressLabel.SetText(fmt.Sprintf("Downloading %s...", f.Name))
		c.SetStatus(fmt.Sprintf("Downloading %s", f.Name))

		go func() {
			written, err := c.client.Download(c.selectedPeer, f.Path, c.downloadDir)
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

			progressBar.Show()
			progressBar.SetValue(0)
			progressLabel.Show()
			progressLabel.SetText(fmt.Sprintf("Uploading %s...", filepath.Base(localPath)))
			c.SetStatus(fmt.Sprintf("Uploading %s", filepath.Base(localPath)))

			go func() {
				written, err := c.client.Upload(c.selectedPeer, localPath, remotePath)
				progressBar.SetValue(1.0)
				if err != nil {
					fileStatus.SetText("Upload error: " + err.Error())
					c.SetStatus("Upload failed")
				} else {
					fileStatus.SetText(fmt.Sprintf("Uploaded %s (%s)", filepath.Base(localPath), formatBytes(written)))
					c.SetStatus("Upload complete")
					refreshFiles()
				}
			}()
		}, w)
		fd.Show()
	})

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

	progressSection := container.NewVBox(progressBar, progressLabel)

	rightPanel := container.NewBorder(
		navBar,
		container.NewVBox(actionBar, progressSection, fileStatus),
		nil, nil,
		fileList,
	)

	peerPanel := container.NewBorder(
		widget.NewLabelWithStyle("Peers", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		peerList,
	)

	split := container.NewHSplit(peerPanel, rightPanel)
	split.SetOffset(0.22)

	return split
}

