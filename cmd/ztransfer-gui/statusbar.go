package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/quantum-encoding/ztransfer/pkg/crypto"
)

// BuildStatusBar creates the bottom status bar.
func (c *Controller) BuildStatusBar() fyne.CanvasObject {
	statusLabel := widget.NewLabel("Ready")
	statusLabel.TextStyle = fyne.TextStyle{Monospace: true}

	// Status dot
	dot := canvas.NewCircle(theme.DisabledColor())
	dot.Resize(fyne.NewSize(8, 8))

	versionLabel := widget.NewLabel(fmt.Sprintf("ztransfer %s · quantum vault %s · Quantum Encoding Ltd", appVersion, crypto.Version()))
	versionLabel.TextStyle = fyne.TextStyle{Monospace: true}

	// Auto-update status
	go func() {
		for {
			time.Sleep(500 * time.Millisecond)
			status := c.GetStatus()
			running := c.IsServerRunning()
			hasPeer := c.selectedPeer != ""
			fyne.Do(func() {
				statusLabel.SetText(status)
				if running {
					dot.FillColor = theme.SuccessColor()
				} else if hasPeer {
					dot.FillColor = theme.PrimaryColor()
				} else {
					dot.FillColor = theme.DisabledColor()
				}
				dot.Refresh()
			})
		}
	}()

	return container.NewHBox(
		container.NewWithoutLayout(dot),
		statusLabel,
		layout.NewSpacer(),
		versionLabel,
	)
}
