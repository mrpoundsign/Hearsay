package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// AppVersion is injected at build time via -ldflags.
var AppVersion = "v0.0.2"

var (
	// Standard Hearsay Palette: Obsidian Slate + Chrome White + Emerald Mint
	colorObsidianBg    = color.NRGBA{R: 0x0F, G: 0x17, B: 0x2A, A: 0xFF} // #0F172A
	colorSlateSurface  = color.NRGBA{R: 0x1E, G: 0x29, B: 0x3B, A: 0xFF} // #1E293B
	colorSlateInput    = color.NRGBA{R: 0x33, G: 0x41, B: 0x55, A: 0xFF} // #334155
	colorChromeWhite   = color.NRGBA{R: 0xF8, G: 0xFA, B: 0xFC, A: 0xFF} // #F8FAFC
	colorEmeraldMint   = color.NRGBA{R: 0x34, G: 0xD3, B: 0x99, A: 0xFF} // #34D399
	colorEmeraldHover  = color.NRGBA{R: 0x10, G: 0xB9, B: 0x81, A: 0xFF} // #10B981
	colorDangerRed     = color.NRGBA{R: 0xEF, G: 0x44, B: 0x44, A: 0xFF} // #EF4444
	colorMutedSlate    = color.NRGBA{R: 0x94, G: 0xA3, B: 0xB8, A: 0xFF} // #94A3B8

	// High-Contrast Accessibility Palette
	colorHighContrastBg      = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
	colorHighContrastSurface = color.NRGBA{R: 0x12, G: 0x12, B: 0x12, A: 0xFF}
	colorHighContrastFg      = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	colorHighContrastPrimary = color.NRGBA{R: 0x00, G: 0xFF, B: 0x9D, A: 0xFF}
	colorHighContrastFocus   = color.NRGBA{R: 0xFF, G: 0xD7, B: 0x00, A: 0xFF}
)

// HearsayTheme implements fyne.Theme with scalable typography (100%–200%) and High-Contrast mode.
type HearsayTheme struct {
	FontScale    float32
	HighContrast bool
}

// NewHearsayTheme creates a new HearsayTheme with the given font scale and high-contrast flag.
func NewHearsayTheme(fontScale float32, highContrast bool) *HearsayTheme {
	if fontScale < 0.8 || fontScale > 2.5 {
		fontScale = 1.0
	}
	return &HearsayTheme{
		FontScale:    fontScale,
		HighContrast: highContrast,
	}
}

func (t *HearsayTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if t.HighContrast {
		switch name {
		case theme.ColorNameBackground:
			return colorHighContrastBg
		case theme.ColorNameButton, theme.ColorNameInputBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
			return colorHighContrastSurface
		case theme.ColorNameForeground:
			return colorHighContrastFg
		case theme.ColorNamePrimary:
			return colorHighContrastPrimary
		case theme.ColorNameFocus, theme.ColorNameSelection:
			return colorHighContrastFocus
		case theme.ColorNameError:
			return colorDangerRed
		}
	}

	switch name {
	case theme.ColorNameBackground:
		return colorObsidianBg
	case theme.ColorNameButton, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return colorSlateSurface
	case theme.ColorNameInputBackground:
		return colorSlateInput
	case theme.ColorNameForeground:
		return colorChromeWhite
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return colorMutedSlate
	case theme.ColorNamePrimary, theme.ColorNameFocus:
		return colorEmeraldMint
	case theme.ColorNameHover:
		return colorEmeraldHover
	case theme.ColorNameError:
		return colorDangerRed
	default:
		return theme.DefaultTheme().Color(name, theme.VariantDark)
	}
}

func (t *HearsayTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *HearsayTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *HearsayTheme) Size(name fyne.ThemeSizeName) float32 {
	base := theme.DefaultTheme().Size(name)
	scale := t.FontScale
	if scale <= 0 {
		scale = 1.0
	}
	switch name {
	case theme.SizeNameText,
		theme.SizeNameHeadingText,
		theme.SizeNameSubHeadingText,
		theme.SizeNameCaptionText:
		return base * scale
	default:
		return base
	}
}
