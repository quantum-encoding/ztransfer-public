//go:build darwin || linux

package main

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/quantum-encoding/ztransfer/pkg/remote"
)

// BuildRemoteTab creates the remote access interface with three modes:
// Terminal (PTY shell), Viewer (live screen view), and Computer Use (AI).
func (c *Controller) BuildRemoteTab(w fyne.Window) fyne.CanvasObject {
	// === Connection panel ===
	codeEntry := widget.NewEntry()
	codeEntry.SetPlaceHolder("warp-429-delta")

	modeSelect := widget.NewSelect(
		[]string{"Terminal", "Viewer (Control)", "Viewer (Watch)", "Computer Use (AI)"},
		nil,
	)
	modeSelect.SetSelected("Viewer (Control)")

	statusLabel := widget.NewLabel("Not connected")
	statusLabel.TextStyle = fyne.TextStyle{Monospace: true}
	statusDot := canvas.NewCircle(theme.DisabledColor())
	statusDot.Resize(fyne.NewSize(10, 10))

	// === Session info ===
	peerLabel := widget.NewLabel("—")
	durationLabel := widget.NewLabel("—")
	resLabel := widget.NewLabel("—")
	fpsLabel := widget.NewLabel("—")
	sizeLabel := widget.NewLabel("—")

	// === Screen viewer ===
	screenImage := canvas.NewImageFromResource(nil)
	screenImage.FillMode = canvas.ImageFillContain
	screenImage.SetMinSize(fyne.NewSize(800, 500))

	// === Activity log ===
	activityLog := widget.NewMultiLineEntry()
	activityLog.Disable()
	activityLog.TextStyle = fyne.TextStyle{Monospace: true}
	activityLog.SetMinRowsVisible(6)

	logLine := func(msg string) {
		ts := time.Now().Format("15:04:05")
		fyne.Do(func() {
			activityLog.SetText(activityLog.Text + fmt.Sprintf("[%s] %s\n", ts, msg))
		})
	}

	// === State ===
	var (
		activeClient  *remote.ComputerClient
		activeSession *remote.Session
		stopViewer    chan struct{}
	)

	setConnected := func(connected bool, peer string) {
		fyne.Do(func() {
			if connected {
				statusDot.FillColor = theme.SuccessColor()
				statusLabel.SetText("Connected to " + peer)
				peerLabel.SetText(peer)
			} else {
				statusDot.FillColor = theme.DisabledColor()
				statusLabel.SetText("Disconnected")
				peerLabel.SetText("—")
				durationLabel.SetText("—")
				resLabel.SetText("—")
				fpsLabel.SetText("—")
				sizeLabel.SetText("—")
			}
			statusDot.Refresh()
		})
	}

	// === Disconnect ===
	disconnectBtn := widget.NewButtonWithIcon("Disconnect", theme.CancelIcon(), func() {
		if stopViewer != nil {
			close(stopViewer)
			stopViewer = nil
		}
		if activeClient != nil {
			activeClient.Close()
			activeClient = nil
		}
		if activeSession != nil {
			activeSession.Close()
			activeSession = nil
		}
		setConnected(false, "")
		logLine("Disconnected")
	})
	disconnectBtn.Importance = widget.DangerImportance

	// === Connect ===
	connectBtn := widget.NewButtonWithIcon("Connect", theme.ConfirmIcon(), func() {
		code := strings.TrimSpace(codeEntry.Text)
		if code == "" {
			statusLabel.SetText("Enter a warp code")
			return
		}

		mode := modeSelect.Selected
		logLine(fmt.Sprintf("Connecting to %s (%s)...", code, mode))

		go func() {
			fyne.Do(func() {
				statusDot.FillColor = theme.WarningColor()
				statusDot.Refresh()
				statusLabel.SetText("Connecting...")
			})

			session, err := remote.ConnectSession(c.identity, code, "")
			if err != nil {
				logLine("Connection failed: " + err.Error())
				setConnected(false, "")
				return
			}

			activeSession = session
			logLine(fmt.Sprintf("Connected to %s", session.PeerName))
			setConnected(true, session.PeerName)

			switch mode {
			case "Terminal":
				logLine("Terminal mode — use the CLI for interactive shell")
				// Fyne doesn't have a terminal widget, so we point the user to CLI
				fyne.Do(func() {
					statusLabel.SetText("Connected — run 'ztransfer remote shell " + code + "' in terminal")
				})

			case "Viewer (Control)", "Viewer (Watch)":
				client := remote.NewComputerClient(session.Tunnel)
				activeClient = client
				stopViewer = make(chan struct{})
				viewOnly := mode == "Viewer (Watch)"
				startTime := time.Now()
				var frameCount int
				var totalBytes int64

				logLine(fmt.Sprintf("Viewer started (%s)", mode))

				if info := client.Info; info.Width > 0 {
					fyne.Do(func() {
						resLabel.SetText(fmt.Sprintf("%dx%d (scale %.0f)", info.Width, info.Height, info.Scale))
					})
				}

				// Screenshot loop
				go func() {
					ticker := time.NewTicker(500 * time.Millisecond)
					defer ticker.Stop()

					for {
						select {
						case <-stopViewer:
							return
						case <-ticker.C:
						}

						data, err := client.Screenshot(remote.ScreenRequest{
							Format:  "jpeg",
							Quality: 65,
							Scale:   2,
						})
						if err != nil {
							logLine("Screenshot error: " + err.Error())
							continue
						}

						frameCount++
						totalBytes += int64(len(data))

						res := fyne.NewStaticResource("screen.jpg", data)
						fyne.Do(func() {
							screenImage.Resource = res
							screenImage.Refresh()

							elapsed := time.Since(startTime).Seconds()
							if elapsed > 0 {
								fpsLabel.SetText(fmt.Sprintf("%.1f fps", float64(frameCount)/elapsed))
							}
							sizeLabel.SetText(fmt.Sprintf("%s total", formatBytes(totalBytes)))
							durationLabel.SetText(time.Since(startTime).Truncate(time.Second).String())
						})
					}
				}()

				// TODO: mouse/keyboard forwarding from Fyne canvas
				_ = viewOnly

			case "Computer Use (AI)":
				client := remote.NewComputerClient(session.Tunnel)
				activeClient = client
				logLine("Computer Use session started — use REST API to control")
				fyne.Do(func() {
					statusLabel.SetText("AI session active — API at localhost:9877")
				})
			}
		}()
	})
	connectBtn.Importance = widget.HighImportance

	// === Layout ===
	connectionForm := container.NewVBox(
		widget.NewLabelWithStyle("Remote Connection", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Warp Code:"), codeEntry,
			widget.NewLabel("Mode:"), modeSelect,
		),
		container.NewHBox(connectBtn, disconnectBtn, layout.NewSpacer(),
			container.NewWithoutLayout(statusDot), statusLabel),
	)

	sessionInfo := container.NewVBox(
		widget.NewLabelWithStyle("Session", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Peer:"), peerLabel,
			widget.NewLabel("Duration:"), durationLabel,
			widget.NewLabel("Resolution:"), resLabel,
			widget.NewLabel("Frame Rate:"), fpsLabel,
			widget.NewLabel("Transfer:"), sizeLabel,
		),
	)

	rightPanel := container.NewVBox(
		connectionForm,
		widget.NewSeparator(),
		sessionInfo,
	)

	// Viewer panel in center
	viewerPanel := container.NewBorder(
		widget.NewLabelWithStyle("Screen", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		container.NewStack(screenImage),
	)

	// Log at bottom
	logPanel := container.NewBorder(
		widget.NewLabelWithStyle("Activity", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		container.NewVScroll(activityLog),
	)

	// Main layout: viewer left, controls right, log bottom
	mainArea := container.NewHSplit(viewerPanel, rightPanel)
	mainArea.SetOffset(0.7)

	vsplit := container.NewVSplit(mainArea, logPanel)
	vsplit.SetOffset(0.75)

	return vsplit
}
