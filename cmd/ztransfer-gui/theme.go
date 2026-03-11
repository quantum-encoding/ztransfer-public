package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// ztransferTheme implements a dual-mode theme with teal accent.
type ztransferTheme struct {
	variant fyne.ThemeVariant // 0 = system, 1 = dark, 2 = light
}

func (t *ztransferTheme) resolveVariant(variant fyne.ThemeVariant) fyne.ThemeVariant {
	if t.variant == theme.VariantDark || t.variant == theme.VariantLight {
		return t.variant
	}
	return variant // system default
}

func (t *ztransferTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	v := t.resolveVariant(variant)

	// Teal accent shared across both modes
	teal := color.NRGBA{R: 34, G: 211, B: 167, A: 255}       // #22d3a7
	tealFocus := color.NRGBA{R: 34, G: 211, B: 167, A: 100}

	switch name {
	case theme.ColorNamePrimary:
		return teal
	case theme.ColorNameFocus:
		return tealFocus
	case theme.ColorNameError:
		return color.NRGBA{R: 239, G: 68, B: 68, A: 255}  // #ef4444
	case theme.ColorNameSuccess:
		return color.NRGBA{R: 34, G: 197, B: 94, A: 255}  // #22c55e
	case theme.ColorNameWarning:
		return color.NRGBA{R: 245, G: 158, B: 11, A: 255} // #f59e0b
	}

	if v == theme.VariantDark {
		return t.darkColor(name)
	}
	return t.lightColor(name)
}

func (t *ztransferTheme) darkColor(name fyne.ThemeColorName) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 10, G: 12, B: 16, A: 255} // #0a0c10
	case theme.ColorNameButton:
		return color.NRGBA{R: 21, G: 26, B: 35, A: 255} // #151a23
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 15, G: 18, B: 24, A: 255}
	case theme.ColorNameHover:
		return color.NRGBA{R: 30, G: 37, B: 56, A: 255} // #1e2538
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 15, G: 18, B: 24, A: 255} // #0f1218
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 30, G: 37, B: 56, A: 255}
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 30, G: 37, B: 56, A: 255}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 226, G: 232, B: 240, A: 255} // #e2e8f0
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 90, G: 100, B: 120, A: 255}
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 90, G: 100, B: 120, A: 255}
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 15, G: 18, B: 24, A: 255}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 15, G: 18, B: 24, A: 255}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 10, G: 12, B: 16, A: 240}
	}
	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

func (t *ztransferTheme) lightColor(name fyne.ThemeColorName) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 248, G: 250, B: 252, A: 255} // #f8fafc
	case theme.ColorNameButton:
		return color.NRGBA{R: 241, G: 245, B: 249, A: 255} // #f1f5f9
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 226, G: 232, B: 240, A: 255}
	case theme.ColorNameHover:
		return color.NRGBA{R: 226, G: 232, B: 240, A: 255} // #e2e8f0
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255} // white
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 203, G: 213, B: 225, A: 255} // #cbd5e1
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 226, G: 232, B: 240, A: 255} // #e2e8f0
	case theme.ColorNameForeground:
		return color.NRGBA{R: 15, G: 23, B: 42, A: 255} // #0f172a
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 148, G: 163, B: 184, A: 255} // #94a3b8
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 148, G: 163, B: 184, A: 255}
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 241, G: 245, B: 249, A: 255}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 248, G: 250, B: 252, A: 240}
	}
	return theme.DefaultTheme().Color(name, theme.VariantLight)
}

func (t *ztransferTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *ztransferTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *ztransferTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 4
	case theme.SizeNameText:
		return 13
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return theme.DefaultTheme().Size(name)
}
