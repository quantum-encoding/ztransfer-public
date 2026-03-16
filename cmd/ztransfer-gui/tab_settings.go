package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/quantum-encoding/ztransfer/pkg/crypto"
)

// BuildSettingsTab creates the settings interface with Audit and Tokens
// folded in as sub-tabs under an "Advanced" section.
func (c *Controller) BuildSettingsTab(a fyne.App, w fyne.Window) fyne.CanvasObject {
	// Default download directory
	downloadEntry := widget.NewEntry()
	downloadEntry.SetText(c.downloadDir)
	downloadEntry.OnChanged = func(s string) {
		c.downloadDir = s
	}

	// Default port
	portEntry := widget.NewEntry()
	portEntry.SetText("9876")

	// Theme selector
	themeSelect := widget.NewSelect([]string{"Dark", "Light", "System"}, func(s string) {
		switch s {
		case "Dark":
			a.Settings().SetTheme(&ztransferTheme{variant: theme.VariantDark})
		case "Light":
			a.Settings().SetTheme(&ztransferTheme{variant: theme.VariantLight})
		case "System":
			a.Settings().SetTheme(&ztransferTheme{})
		}
	})
	themeSelect.SetSelected("Dark")

	// Identity info
	identitySection := panelWithTitle("Identity", container.New(layout.NewFormLayout(),
		widget.NewLabel("Name:"), selectableLabel(c.identity.Name),
		widget.NewLabel("Fingerprint:"), selectableLabelMono(c.identity.Fingerprint()),
		widget.NewLabel("Algorithm:"), selectableLabel("ML-DSA-65 (FIPS 204)"),
		widget.NewLabel("Key Size:"), selectableLabel("1952 bytes (public) / 4032 bytes (secret)"),
	))

	// About section
	aboutSection := panelWithTitle("About", container.NewVBox(
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Version:"), selectableLabel(appVersion),
			widget.NewLabel("Quantum Vault:"), selectableLabel(crypto.Version()),
			widget.NewLabel("Transport:"), selectableLabel("TLS 1.3 (hardware-accelerated)"),
			widget.NewLabel("Auth:"), selectableLabel("ML-DSA-65 + TOFU pairing"),
			widget.NewLabel("Key Exchange:"), selectableLabel("Hybrid ML-KEM-768 + X25519"),
		),
		widget.NewSeparator(),
		widget.NewLabel("Quantum Encoding Ltd"),
	))

	// Preferences section
	prefsSection := panelWithTitle("Preferences", container.New(layout.NewFormLayout(),
		widget.NewLabel("Download Dir:"), downloadEntry,
		widget.NewLabel("Default Port:"), portEntry,
		widget.NewLabel("Theme:"), themeSelect,
	))

	settingsPanels := container.NewAppTabs(
		container.NewTabItem("Preferences", container.NewVBox(prefsSection)),
		container.NewTabItem("Identity", container.NewVBox(identitySection)),
		container.NewTabItem("About", container.NewVBox(aboutSection)),
		container.NewTabItem("Guide", c.BuildGuideTab()),
		container.NewTabItem("Audit Logs", c.BuildAuditTab(w)),
		container.NewTabItem("Login", c.BuildTokensTab()),
	)
	settingsPanels.SetTabLocation(container.TabLocationTop)

	return settingsPanels
}
