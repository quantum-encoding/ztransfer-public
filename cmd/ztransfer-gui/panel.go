package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// readOnlyEntry is a widget.Entry that allows text selection and copying
// but rejects keyboard input. Fyne's Disable() blocks all interaction
// including selection, so this custom widget intercepts key events instead.
type readOnlyEntry struct {
	widget.Entry
}

func newReadOnlyEntry(text string) *readOnlyEntry {
	e := &readOnlyEntry{}
	e.ExtendBaseWidget(e)
	e.SetText(text)
	return e
}

func newReadOnlyEntryMono(text string) *readOnlyEntry {
	e := &readOnlyEntry{}
	e.ExtendBaseWidget(e)
	e.TextStyle = fyne.TextStyle{Monospace: true}
	e.SetText(text)
	return e
}

func newReadOnlyMultiLine(text string) *readOnlyEntry {
	e := &readOnlyEntry{}
	e.MultiLine = true
	e.Wrapping = fyne.TextWrapBreak
	e.ExtendBaseWidget(e)
	e.SetText(text)
	return e
}

func newReadOnlyMultiLineMono(text string) *readOnlyEntry {
	e := &readOnlyEntry{}
	e.MultiLine = true
	e.Wrapping = fyne.TextWrapBreak
	e.TextStyle = fyne.TextStyle{Monospace: true}
	e.ExtendBaseWidget(e)
	e.SetText(text)
	return e
}

// TypedRune ignores character input — makes the entry read-only.
func (e *readOnlyEntry) TypedRune(_ rune) {}

// TypedKey allows only selection/copy shortcuts and rejects editing keys.
func (e *readOnlyEntry) TypedKey(ev *fyne.KeyEvent) {
	// Allow cursor movement for selection
	switch ev.Name {
	case fyne.KeyLeft, fyne.KeyRight, fyne.KeyUp, fyne.KeyDown,
		fyne.KeyHome, fyne.KeyEnd, fyne.KeyPageUp, fyne.KeyPageDown:
		e.Entry.TypedKey(ev)
	}
	// Block Delete, Backspace, Return, Tab, etc.
}

// TypedShortcut allows copy (Ctrl/Cmd+C) and select-all (Ctrl/Cmd+A)
// but blocks cut, paste, and other editing shortcuts.
func (e *readOnlyEntry) TypedShortcut(s fyne.Shortcut) {
	switch s.(type) {
	case *fyne.ShortcutCopy, *fyne.ShortcutSelectAll:
		e.Entry.TypedShortcut(s)
	}
}

// Convenience constructors that return *widget.Entry for compatibility
// with existing callers that use .SetText(), .Text, etc.

func selectableLabel(text string) *widget.Entry {
	return &newReadOnlyEntry(text).Entry
}

func selectableLabelMono(text string) *widget.Entry {
	return &newReadOnlyEntryMono(text).Entry
}

func selectableMultiLine(text string) *widget.Entry {
	return &newReadOnlyMultiLine(text).Entry
}

func selectableMultiLineMono(text string) *widget.Entry {
	return &newReadOnlyMultiLineMono(text).Entry
}

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
