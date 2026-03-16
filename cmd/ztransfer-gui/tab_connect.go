//go:build darwin || linux

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/quantum-encoding/ztransfer/pkg/auth"
	"github.com/quantum-encoding/ztransfer/pkg/server"
)

// BuildConnectTab creates the unified connection tab that merges
// the old Server and Peers tabs into a single, friendly interface.
func (c *Controller) BuildConnectTab(w fyne.Window) fyne.CanvasObject {
	// ─── Share Section (was Server tab) ───

	dirLabel := widget.NewLabelWithStyle(c.serverDir, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	browseButton := widget.NewButtonWithIcon("Browse", theme.FolderOpenIcon(), func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			c.serverDir = uri.Path()
			dirLabel.SetText(c.serverDir)
		}, w)
		fd.Resize(fyne.NewSize(800, 560))
		fd.Show()
	})

	// Token display — large and prominent, selectable
	tokenLabel := selectableLabelMono("—")

	copyTokenBtn := widget.NewButtonWithIcon("Copy Token", theme.ContentCopyIcon(), func() {
		if tokenLabel.Text != "" && tokenLabel.Text != "—" {
			w.Clipboard().SetContent(tokenLabel.Text)
			c.SetStatus("Token copied to clipboard")
		}
	})

	// Address display — selectable
	addressLabel := selectableLabelMono("Not running")

	// Connection log
	logText := widget.NewMultiLineEntry()
	logText.Disable()
	logText.SetMinRowsVisible(5)
	logText.TextStyle = fyne.TextStyle{Monospace: true}

	appendLog := func(msg string) {
		ts := time.Now().Format("15:04:05")
		logText.SetText(logText.Text + fmt.Sprintf("[%s] %s\n", ts, msg))
		logText.CursorRow = len(logText.Text)
	}

	// Start/Stop button
	var toggleButton *widget.Button
	toggleButton = widget.NewButtonWithIcon("Start Sharing", theme.MediaPlayIcon(), func() {
		if c.IsServerRunning() {
			c.mu.Lock()
			if c.serverCancel != nil {
				c.serverCancel()
			}
			c.serverRunning = false
			c.mu.Unlock()

			toggleButton.SetText("Start Sharing")
			toggleButton.SetIcon(theme.MediaPlayIcon())
			toggleButton.Importance = widget.HighImportance
			addressLabel.SetText("Not running")
			tokenLabel.SetText("—")
			c.SetStatus("Server stopped")
			appendLog("Server stopped")
		} else {
			token, err := auth.GeneratePairToken()
			if err != nil {
				appendLog("Error generating token: " + err.Error())
				return
			}

			c.mu.Lock()
			c.pairToken = token
			c.mu.Unlock()

			tokenLabel.SetText(token)

			port := c.serverPort
			var srv *server.Server
			for attempt := 0; attempt < 10; attempt++ {
				ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
				if err != nil {
					appendLog(fmt.Sprintf("Port %d in use, trying %d...", port, port+1))
					port++
					continue
				}
				ln.Close()
				srv = &server.Server{
					RootDir:   c.serverDir,
					Identity:  c.identity,
					PeerStore: c.peerStore,
					PairToken: token,
					Port:      port,
				}
				break
			}
			if srv == nil {
				appendLog(fmt.Sprintf("Could not find a free port (%d-%d)", c.serverPort, port-1))
				return
			}

			if port != c.serverPort {
				appendLog(fmt.Sprintf("Using port %d (original %d was busy)", port, c.serverPort))
				c.serverPort = port
			}

			ctx, cancel := context.WithCancel(context.Background())
			c.mu.Lock()
			c.serverCancel = cancel
			c.serverRunning = true
			c.mu.Unlock()

			addrs := localAddresses()
			if len(addrs) > 0 {
				addressLabel.SetText(fmt.Sprintf("https://%s:%d", addrs[0], port))
			}

			toggleButton.SetText("Stop Sharing")
			toggleButton.SetIcon(theme.MediaStopIcon())
			toggleButton.Importance = widget.DangerImportance
			c.SetStatus(fmt.Sprintf("Serving %s on port %d", c.serverDir, port))
			appendLog(fmt.Sprintf("Server started on port %d", port))
			appendLog(fmt.Sprintf("Pair token: %s — share this with the other person", token))

			go func() {
				err := srv.Start()
				if ctx.Err() == nil && err != nil {
					fyne.Do(func() {
						appendLog("Server error: " + err.Error())
					})
				}
			}()

			_ = ctx
		}
	})
	toggleButton.Importance = widget.HighImportance

	shareSection := panelWithTitle("Share From This Machine", container.NewVBox(
		container.NewHBox(
			widget.NewLabel("Folder:"),
			dirLabel,
			layout.NewSpacer(),
			browseButton,
		),
		widget.NewSeparator(),
		container.NewHBox(layout.NewSpacer(), toggleButton, layout.NewSpacer()),
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewLabel("Address:"),
			addressLabel,
		),
		container.NewHBox(
			widget.NewLabel("Pair Token:"),
			tokenLabel,
			copyTokenBtn,
		),
	))

	// ─── Peers Section (was Peers tab) ───

	nameValue := selectableLabel("—")
	addressValue := selectableLabelMono("—")
	pairedValue := selectableLabel("—")

	peerList := widget.NewList(
		func() int { return len(c.peerStore.ListPeers()) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.AccountIcon()),
				container.NewVBox(
					widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					widget.NewLabel(""),
				),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			peers := c.peerStore.ListPeers()
			if id >= len(peers) {
				return
			}
			p := peers[id]
			box := obj.(*fyne.Container)
			inner := box.Objects[1].(*fyne.Container)
			inner.Objects[0].(*widget.Label).SetText(p.Name)
			inner.Objects[1].(*widget.Label).SetText(p.Address)
		},
	)

	peerList.OnSelected = func(id widget.ListItemID) {
		peers := c.peerStore.ListPeers()
		if id >= len(peers) {
			return
		}
		p := peers[id]
		nameValue.SetText(p.Name)
		addressValue.SetText(p.Address)
		pairedValue.SetText(p.PairedAt.Format("2006-01-02 15:04"))
	}

	removeButton := widget.NewButtonWithIcon("Remove", theme.DeleteIcon(), func() {
		selected := nameValue.Text
		if selected == "" || selected == "—" {
			return
		}
		dialog.ShowConfirm("Remove Peer", fmt.Sprintf("Remove %s from paired machines?", selected), func(ok bool) {
			if !ok {
				return
			}
			c.peerStore.RemovePeer(selected)
			peerList.Refresh()
			nameValue.SetText("—")
			addressValue.SetText("—")
			pairedValue.SetText("—")
		}, w)
	})
	removeButton.Importance = widget.DangerImportance

	// Pair new peer form
	addrEntry := widget.NewEntry()
	addrEntry.SetPlaceHolder("192.168.1.50:9876")
	tokenEntry := widget.NewEntry()
	tokenEntry.SetPlaceHolder("Paste token here")

	pairStatus := widget.NewLabel("")

	pairButton := widget.NewButtonWithIcon("Pair", theme.ConfirmIcon(), func() {
		addr := addrEntry.Text
		tok := tokenEntry.Text
		if addr == "" || tok == "" {
			pairStatus.SetText("Enter address and token")
			return
		}

		pairStatus.SetText("Pairing...")
		go func() {
			err := auth.RequestPair(addr, tok, c.identity, c.peerStore)
			fyne.Do(func() {
				if err != nil {
					pairStatus.SetText("Failed: " + err.Error())
				} else {
					pairStatus.SetText("Paired successfully!")
					peerList.Refresh()
					addrEntry.SetText("")
					tokenEntry.SetText("")
				}
			})
		}()
	})
	pairButton.Importance = widget.HighImportance

	peerDetail := container.New(layout.NewFormLayout(),
		widget.NewLabel("Name:"), nameValue,
		widget.NewLabel("Address:"), addressValue,
		widget.NewLabel("Paired:"), pairedValue,
	)

	pairForm := container.NewVBox(
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Address:"), addrEntry,
			widget.NewLabel("Token:"), tokenEntry,
		),
		container.NewHBox(pairButton, pairStatus),
	)

	peersSection := panelWithTitle("Paired Machines", container.NewVBox(
		peerList,
		widget.NewSeparator(),
		peerDetail,
		removeButton,
	))

	pairSection := panelWithTitle("Pair New Machine", pairForm)

	// ─── Layout ───

	leftPanel := container.NewVBox(
		shareSection,
		logText,
	)

	rightPanel := container.NewVBox(
		peersSection,
		pairSection,
	)

	split := container.NewHSplit(leftPanel, rightPanel)
	split.SetOffset(0.5)

	return split
}

func localAddresses() []string {
	var result []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return result
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			result = append(result, ipnet.IP.String())
		}
	}
	if len(result) == 0 {
		result = append(result, "127.0.0.1")
	}
	return result
}

var _ = os.Getenv
