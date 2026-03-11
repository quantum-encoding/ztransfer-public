package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/quantum-encoding/ztransfer-public/pkg/crypto"
)

// BuildSettingsTab creates the settings interface.
func (c *Controller) BuildSettingsTab(a fyne.App) fyne.CanvasObject {
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
	identitySection := container.NewVBox(
		widget.NewLabelWithStyle("Identity", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Name:"), widget.NewLabel(c.identity.Name),
			widget.NewLabel("Fingerprint:"), widget.NewLabelWithStyle(c.identity.Fingerprint(), fyne.TextAlignLeading, fyne.TextStyle{Monospace: true}),
			widget.NewLabel("Algorithm:"), widget.NewLabel("ML-DSA-65 (FIPS 204)"),
			widget.NewLabel("Key Size:"), widget.NewLabel("1952 bytes (public) / 4032 bytes (secret)"),
		),
	)

	// About section
	aboutSection := container.NewVBox(
		widget.NewLabelWithStyle("About", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Version:"), widget.NewLabel(appVersion),
			widget.NewLabel("Quantum Vault:"), widget.NewLabel(crypto.Version()),
			widget.NewLabel("Transport:"), widget.NewLabel("TLS 1.3 (hardware-accelerated)"),
			widget.NewLabel("Auth:"), widget.NewLabel("ML-DSA-65 + TOFU pairing"),
			widget.NewLabel("Key Exchange:"), widget.NewLabel("Hybrid ML-KEM-768 + X25519"),
		),
		widget.NewSeparator(),
		widget.NewLabel("Quantum Encoding Ltd"),
	)

	// Preferences section
	prefsSection := container.NewVBox(
		widget.NewLabelWithStyle("Preferences", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Download Dir:"), downloadEntry,
			widget.NewLabel("Default Port:"), portEntry,
			widget.NewLabel("Theme:"), themeSelect,
		),
	)

	return container.NewVBox(
		prefsSection,
		identitySection,
		aboutSection,
	)
}
