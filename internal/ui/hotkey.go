package ui

import (
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
)

const (
	ModAlt      uint32 = 0x0001
	ModControl  uint32 = 0x0002
	ModShift    uint32 = 0x0004
	ModNoRepeat uint32 = 0x4000

	HotkeyIDPauseResume uintptr = 1
	HotkeyIDSkipCurrent uintptr = 2
	HotkeyIDReplayLast  uintptr = 3
	HotkeyIDPanicMute   uintptr = 4
)

// HotkeySpec represents a parsed hotkey combination.
type HotkeySpec struct {
	Ctrl   bool
	Shift  bool
	Alt    bool
	Key    string
	WinMod uint32
	WinVK  uint32
}

func (h HotkeySpec) String() string {
	var parts []string
	if h.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if h.Shift {
		parts = append(parts, "Shift")
	}
	if h.Alt {
		parts = append(parts, "Alt")
	}
	if h.Key != "" {
		parts = append(parts, h.Key)
	}
	return strings.Join(parts, "+")
}

// ParseHotkey parses a hotkey string like "Ctrl+Shift+M" or "F9" into a HotkeySpec.
func ParseHotkey(raw string) (HotkeySpec, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return HotkeySpec{}, errors.New("hotkey cannot be empty")
	}

	var spec HotkeySpec
	for p := range strings.SplitSeq(trimmed, "+") {
		token := strings.TrimSpace(p)
		if token == "" {
			continue
		}
		switch strings.ToLower(token) {
		case "ctrl", "control":
			spec.Ctrl = true
		case "shift":
			spec.Shift = true
		case "alt":
			spec.Alt = true
		default:
			if spec.Key != "" {
				return HotkeySpec{}, fmt.Errorf("multiple non-modifier keys in %q", raw)
			}
			canonical, vk, ok := resolveKeyNameAndVK(token)
			if !ok {
				return HotkeySpec{}, fmt.Errorf("unsupported hotkey key %q", token)
			}
			spec.Key = canonical
			spec.WinVK = vk
		}
	}

	if spec.Key == "" {
		return HotkeySpec{}, fmt.Errorf("hotkey %q must include a non-modifier key", raw)
	}
	if len(spec.Key) == 1 && !spec.Ctrl && !spec.Shift && !spec.Alt {
		return HotkeySpec{}, fmt.Errorf("key %q requires at least one modifier (Ctrl, Shift, or Alt)", spec.Key)
	}

	spec.WinMod = ModNoRepeat
	if spec.Ctrl {
		spec.WinMod |= ModControl
	}
	if spec.Shift {
		spec.WinMod |= ModShift
	}
	if spec.Alt {
		spec.WinMod |= ModAlt
	}
	return spec, nil
}

func resolveKeyNameAndVK(token string) (string, uint32, bool) {
	upper := strings.ToUpper(strings.TrimSpace(token))
	if len(upper) == 1 && upper[0] >= 'A' && upper[0] <= 'Z' {
		return upper, uint32(upper[0]), true
	}
	if len(upper) == 1 && upper[0] >= '0' && upper[0] <= '9' {
		return upper, uint32(upper[0]), true
	}
	for i := 1; i <= 12; i++ {
		fName := fmt.Sprintf("F%d", i)
		if upper == fName {
			return fName, uint32(0x70 + i - 1), true
		}
	}
	switch upper {
	case "SPACE":
		return "Space", 0x20, true
	case "TAB":
		return "Tab", 0x09, true
	case "BACKSPACE", "BACK":
		return "Backspace", 0x08, true
	case "INSERT", "INS":
		return "Insert", 0x2D, true
	case "DELETE", "DEL":
		return "Delete", 0x2E, true
	case "HOME":
		return "Home", 0x24, true
	case "END":
		return "End", 0x23, true
	case "PAGEUP", "PGUP":
		return "PageUp", 0x21, true
	case "PAGEDOWN", "PGDN":
		return "PageDown", 0x22, true
	case "PAUSE":
		return "Pause", 0x13, true
	}
	return "", 0, false
}

// CanonicalKeyFromFyne maps a Fyne KeyName to a canonical key string if supported.
func CanonicalKeyFromFyne(name fyne.KeyName) (string, bool) {
	canonical, _, ok := resolveKeyNameAndVK(string(name))
	return canonical, ok
}
