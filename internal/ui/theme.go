package ui

import (
	"image/color"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// AppVersion is injected at build time via -ldflags.
var AppVersion = "v0.0.3"

// UI scale mode constants and display labels.
const (
	UIScale85   = "0.85"
	UIScale90   = "0.9"
	UIScale100  = "1.0"
	UIScale115  = "1.15"
	UIScale125  = "1.25"
	UIScale150  = "1.5"
	UIScaleAuto = "auto"

	uiScaleLabel85   = "85%"
	uiScaleLabel90   = "90%"
	uiScaleLabel100  = "100%"
	uiScaleLabel115  = "115%"
	uiScaleLabel125  = "125%"
	uiScaleLabel150  = "150%"
	uiScaleLabelAuto = "Auto (OS)"
)

// UIScaleOptions returns the human-readable labels for the UI Scale selector.
func UIScaleOptions() []string {
	return []string{
		uiScaleLabel85,
		uiScaleLabel90,
		uiScaleLabel100,
		uiScaleLabel115,
		uiScaleLabel125,
		uiScaleLabel150,
		uiScaleLabelAuto,
	}
}

// NormalizeUIScale returns a canonical UI scale mode string ("0.85", "0.9", "1.0", "1.15", "1.25", "1.5", or "auto").
func NormalizeUIScale(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case UIScale85, "85%":
		return UIScale85
	case UIScale90, "0.90", "90%":
		return UIScale90
	case UIScale100, "1", "100%":
		return UIScale100
	case UIScale115, "115%":
		return UIScale115
	case UIScale125, "125%":
		return UIScale125
	case UIScale150, "1.50", "150%":
		return UIScale150
	case UIScaleAuto, "auto (os)":
		return UIScaleAuto
	default:
		return UIScale100
	}
}

// FormatUIScaleLabel maps a persisted config UIScale mode to its dropdown label.
func FormatUIScaleLabel(mode string) string {
	switch NormalizeUIScale(mode) {
	case UIScale85:
		return uiScaleLabel85
	case UIScale90:
		return uiScaleLabel90
	case UIScale115:
		return uiScaleLabel115
	case UIScale125:
		return uiScaleLabel125
	case UIScale150:
		return uiScaleLabel150
	case UIScaleAuto:
		return uiScaleLabelAuto
	default:
		return uiScaleLabel100
	}
}

// ParseUIScaleLabel maps a dropdown label back to its persisted config UIScale mode.
func ParseUIScaleLabel(label string) string {
	switch strings.TrimSpace(label) {
	case uiScaleLabel85:
		return UIScale85
	case uiScaleLabel90:
		return UIScale90
	case uiScaleLabel115:
		return UIScale115
	case uiScaleLabel125:
		return UIScale125
	case uiScaleLabel150:
		return UIScale150
	case uiScaleLabelAuto:
		return UIScaleAuto
	default:
		return UIScale100
	}
}

// ApplyUIScale configures Fyne's DPI detection and user scale environment variables,
// and refreshes the active theme and open windows while preserving physical window dimensions.
func ApplyUIScale(appObj fyne.App, scaleMode string, fontScale float32, highContrast bool) {
	mode := NormalizeUIScale(scaleMode)
	if mode == UIScaleAuto {
		_ = os.Setenv("FYNE_DISABLE_DPI_DETECTION", "false")
		_ = os.Setenv("FYNE_SCALE", "auto")
	} else {
		_ = os.Setenv("FYNE_DISABLE_DPI_DETECTION", "true")
		_ = os.Setenv("FYNE_SCALE", mode)
	}
	if appObj == nil {
		return
	}

	type winSnapshot struct {
		win   fyne.Window
		physW float32
		physH float32
	}
	var snaps []winSnapshot
	if drv := appObj.Driver(); drv != nil {
		for _, w := range drv.AllWindows() {
			if w == nil || w.Canvas() == nil {
				continue
			}
			sc := w.Canvas().Scale()
			sz := w.Canvas().Size()
			if sc > 0 && sz.Width > 0 && sz.Height > 0 {
				snaps = append(snaps, winSnapshot{
					win:   w,
					physW: sz.Width * sc,
					physH: sz.Height * sc,
				})
			}
		}
	}

	appObj.Settings().SetTheme(NewHearsayTheme(fontScale, highContrast))

	for _, snap := range snaps {
		newScale := snap.win.Canvas().Scale()
		if newScale <= 0 {
			continue
		}
		snap.win.Resize(fyne.NewSize(snap.physW/newScale, snap.physH/newScale))
		if content := snap.win.Canvas().Content(); content != nil {
			content.Refresh()
		}
	}
}

var (
	// Standard Hearsay Palette: Obsidian Slate + Chrome White + Deep Emerald Primary
	colorObsidianBg    = color.NRGBA{R: 0x0F, G: 0x17, B: 0x2A, A: 0xFF} // #0F172A
	colorSlateSurface  = color.NRGBA{R: 0x1E, G: 0x29, B: 0x3B, A: 0xFF} // #1E293B
	colorSlateInput    = color.NRGBA{R: 0x33, G: 0x41, B: 0x55, A: 0xFF} // #334155
	colorChromeWhite   = color.NRGBA{R: 0xF8, G: 0xFA, B: 0xFC, A: 0xFF} // #F8FAFC
	colorEmeraldPrimary = color.NRGBA{R: 0x04, G: 0x78, B: 0x57, A: 0xFF} // #047857 (WCAG AA 5.5:1 with Chrome White text)
	colorEmeraldFocus   = color.NRGBA{R: 0x34, G: 0xD3, B: 0x99, A: 0xFF} // #34D399 (Crisp focus ring & selection accent)
	colorEmeraldHover   = color.NRGBA{R: 0x05, G: 0x96, B: 0x69, A: 0xFF} // #059669
	colorDangerRed      = color.NRGBA{R: 0xDC, G: 0x26, B: 0x26, A: 0xFF} // #DC2626
	colorMutedSlate     = color.NRGBA{R: 0x94, G: 0xA3, B: 0xB8, A: 0xFF} // #94A3B8

	// High-Contrast Accessibility Palette (WCAG AAA 7:1+ white text on primary buttons)
	colorHighContrastBg      = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
	colorHighContrastSurface = color.NRGBA{R: 0x1A, G: 0x1A, B: 0x1A, A: 0xFF}
	colorHighContrastFg      = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	colorHighContrastPrimary = color.NRGBA{R: 0x06, G: 0x5F, B: 0x46, A: 0xFF} // #065F46 (7.7:1 contrast with #FFFFFF)
	colorHighContrastFocus   = color.NRGBA{R: 0xAE, G: 0x72, B: 0x00, A: 0xFF}
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
	case theme.ColorNamePrimary:
		return colorEmeraldPrimary
	case theme.ColorNameFocus, theme.ColorNameSelection:
		return colorEmeraldFocus
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
