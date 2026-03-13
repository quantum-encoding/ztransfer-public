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

// BuildServerTab creates the server control interface.
func (c *Controller) BuildServerTab(w fyne.Window) fyne.CanvasObject {
	// Server directory
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

	// Port entry — wide enough to show 5 digits without scrolling
	portEntry := widget.NewEntry()
	portEntry.SetText("9876")
	portEntry.OnChanged = func(s string) {
		fmt.Sscanf(s, "%d", &c.serverPort)
	}
	portEntry.Resize(fyne.NewSize(120, portEntry.MinSize().Height))

	// Token display
	tokenLabel := widget.NewLabelWithStyle("—", fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Monospace: true})

	// Connection info
	addressLabel := widget.NewLabelWithStyle("Not running", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	fingerprintLabel := widget.NewLabel(c.identity.Fingerprint())

	// Connection log
	logText := widget.NewMultiLineEntry()
	logText.Disable()
	logText.SetMinRowsVisible(8)

	appendLog := func(msg string) {
		ts := time.Now().Format("15:04:05")
		logText.SetText(logText.Text + fmt.Sprintf("[%s] %s\n", ts, msg))
		logText.CursorRow = len(logText.Text)
	}

	// Pair command display
	pairCommandLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})

	// Start/Stop button
	var toggleButton *widget.Button
	toggleButton = widget.NewButtonWithIcon("Start Server", theme.MediaPlayIcon(), func() {
		if c.IsServerRunning() {
			// Stop
			c.mu.Lock()
			if c.serverCancel != nil {
				c.serverCancel()
			}
			c.serverRunning = false
			c.mu.Unlock()

			toggleButton.SetText("Start Server")
			toggleButton.SetIcon(theme.MediaPlayIcon())
			addressLabel.SetText("Not running")
			tokenLabel.SetText("—")
			pairCommandLabel.SetText("")
			c.SetStatus("Server stopped")
			appendLog("Server stopped")
		} else {
			// Start
			token, err := auth.GeneratePairToken()
			if err != nil {
				appendLog("Error generating token: " + err.Error())
				return
			}

			c.mu.Lock()
			c.pairToken = token
			c.mu.Unlock()

			tokenLabel.SetText(token)

			// Try the configured port, auto-retry up to 10 ports if in use
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
				portEntry.SetText(fmt.Sprintf("%d", port))
				c.serverPort = port
			}

			ctx, cancel := context.WithCancel(context.Background())
			c.mu.Lock()
			c.serverCancel = cancel
			c.serverRunning = true
			c.mu.Unlock()

			// Get local addresses
			addrs := localAddresses()
			if len(addrs) > 0 {
				addressLabel.SetText(fmt.Sprintf("https://%s:%d", addrs[0], port))
				pairCommandLabel.SetText(fmt.Sprintf("ztransfer pair %s:%d --token %s", addrs[0], port, token))
			}

			toggleButton.SetText("Stop Server")
			toggleButton.SetIcon(theme.MediaStopIcon())
			c.SetStatus(fmt.Sprintf("Serving %s on port %d", c.serverDir, port))
			appendLog(fmt.Sprintf("Server started — serving %s on port %d", c.serverDir, port))
			appendLog(fmt.Sprintf("Pair token: %s", token))

			go func() {
				err := srv.Start()
				if ctx.Err() == nil && err != nil {
					fyne.Do(func() {
						appendLog("Server error: " + err.Error())
					})
				}
			}()

			_ = ctx // ctx used for cancel tracking
		}
	})

	// === Layout ===
	configSection := panelWithTitle("Server Configuration", container.NewVBox(
		container.NewHBox(
			widget.NewLabel("Directory:"),
			dirLabel,
			layout.NewSpacer(),
			browseButton,
		),
		container.NewHBox(
			widget.NewLabel("Port:"),
			container.NewGridWrap(fyne.NewSize(120, portEntry.MinSize().Height), portEntry),
			layout.NewSpacer(),
		),
		container.NewHBox(
			widget.NewLabel("Identity:"),
			widget.NewLabel(c.identity.Name),
			layout.NewSpacer(),
			widget.NewLabel("Fingerprint:"),
			fingerprintLabel,
		),
		widget.NewSeparator(),
		container.NewHBox(layout.NewSpacer(), toggleButton, layout.NewSpacer()),
	))

	infoSection := panelWithTitle("Connection Info", container.NewVBox(
		container.NewHBox(widget.NewLabel("Address:"), addressLabel),
		container.NewHBox(widget.NewLabel("Pair Token:"), tokenLabel),
		container.NewHBox(widget.NewLabel("Pair Command:"), pairCommandLabel),
	))

	logSection := panelWithTitle("Connection Log", logText)

	return container.NewVBox(
		configSection,
		infoSection,
		logSection,
	)
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

// Ensure os import is used.
var _ = os.Getenv
