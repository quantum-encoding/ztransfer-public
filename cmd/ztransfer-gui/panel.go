package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// panel wraps content in a bordered container with a subtle background
// tint and rounded border. Adapts to dark/light mode via theme colors.
func panel(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 6
	bg.StrokeWidth = 1
	bg.StrokeColor = panelBorderColor()
	bg.FillColor = panelBgColor()

	padded := container.NewPadded(content)
	return container.NewStack(bg, padded)
}

// panelWithTitle wraps content with a bold title label and border.
func panelWithTitle(title string, content fyne.CanvasObject) fyne.CanvasObject {
	// Use widget.Card-style title approach but with our custom border
	return panel(container.NewBorder(
		panelTitle(title),
		nil, nil, nil,
		content,
	))
}

// panelTitle creates a styled section header.
func panelTitle(title string) fyne.CanvasObject {
	label := canvas.NewText(title, theme.ForegroundColor())
	label.TextSize = 13
	label.TextStyle = fyne.TextStyle{Bold: true}
	return container.NewVBox(label, canvas.NewLine(panelBorderColor()))
}

// panelBorderColor returns the appropriate border color for the current theme.
func panelBorderColor() color.Color {
	// Check if dark mode by comparing background luminance
	bg := theme.BackgroundColor()
	r, g, b, _ := bg.RGBA()
	luminance := (r>>8)*299 + (g>>8)*587 + (b>>8)*114
	// Dark mode: luminance < 50000
	if luminance < 50000 {
		return color.NRGBA{R: 40, G: 48, B: 64, A: 255} // subtle grey edge in dark
	}
	return color.NRGBA{R: 190, G: 200, B: 215, A: 255} // stronger edge in light
}

// panelBgColor returns the panel fill color — very slightly lifted from background.
func panelBgColor() color.Color {
	bg := theme.BackgroundColor()
	r, g, b, _ := bg.RGBA()
	luminance := (r>>8)*299 + (g>>8)*587 + (b>>8)*114
	if luminance < 50000 {
		return color.NRGBA{R: 14, G: 17, B: 22, A: 255} // barely lifted dark
	}
	return color.NRGBA{R: 255, G: 255, B: 255, A: 255} // white card on light bg
}
