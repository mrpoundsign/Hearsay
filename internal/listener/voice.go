package listener

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/gempir/go-twitch-irc/v4"
	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/TownCrier/pkg/chat"
	"github.com/mrpoundsign/TownCrier/pkg/tts"
)

// DefaultSupertonicVoicePool is the fallback voice pool when deterministic chatter voice hashing is active.
var DefaultSupertonicVoicePool = []string{
	"supertonic:f1", "supertonic:f2", "supertonic:f3", "supertonic:f4", "supertonic:f5",
	"supertonic:m1", "supertonic:m2", "supertonic:m3", "supertonic:m4", "supertonic:m5",
}

// DefaultChatterColorPalette is a curated 16-color high-contrast palette for dark themes.
var DefaultChatterColorPalette = []string{
	"#38BDF8", // Electric Sky Blue
	"#F472B6", // Pink
	"#34D399", // Emerald Green
	"#FBBF24", // Amber Gold
	"#A78BFA", // Soft Purple
	"#FB923C", // Orange
	"#2DD4BF", // Teal
	"#F87171", // Coral Red
	"#60A5FA", // Bright Blue
	"#C084FC", // Violet
	"#4ADE80", // Mint Green
	"#FACC15", // Yellow
	"#E879F9", // Fuchsia
	"#22D3EE", // Cyan
	"#FB7185", // Rose
	"#A3E635", // Lime
}

func chatterName(ev chat.Event) string {
	if strings.TrimSpace(ev.DisplayName) != "" {
		return strings.TrimSpace(ev.DisplayName)
	}
	return strings.TrimSpace(ev.UserName)
}

func speakerKey(ev chat.Event) string {
	u := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ev.UserName), "@"))
	if u == "" {
		u = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ev.DisplayName), "@"))
	}
	if u == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(ev.Platform)) + ":" + u
}

// ExtractUserColor extracts a chatter's platform-provided hex color from chat.Event.Raw when available
// (e.g. Twitch IRC PrivateMessage.User.Color or Kick Pusher sender.identity.color).
func ExtractUserColor(ev chat.Event) string {
	if ev.Raw == nil {
		return ""
	}
	switch raw := ev.Raw.(type) {
	case twitch.PrivateMessage:
		return normalizeHexColor(raw.User.Color)
	case *twitch.PrivateMessage:
		if raw != nil {
			return normalizeHexColor(raw.User.Color)
		}
	case map[string]any:
		if sender, ok := raw["sender"].(map[string]any); ok {
			if ident, ok := sender["identity"].(map[string]any); ok {
				if col, ok := ident["color"].(string); ok {
					return normalizeHexColor(col)
				}
			}
		}
		if msgObj, ok := raw["message"].(map[string]any); ok {
			if sender, ok := msgObj["sender"].(map[string]any); ok {
				if ident, ok := sender["identity"].(map[string]any); ok {
					if col, ok := ident["color"].(string); ok {
						return normalizeHexColor(col)
					}
				}
			}
		}
	}
	return ""
}

// ResolveChatterColorHex returns a dark-theme-legible #RRGGBB hex color for a chatter.
// If rawHex is provided by the platform, it is lightness-clamped so dark colors (e.g. #0000FF) remain readable.
// Otherwise a deterministic color is selected from DefaultChatterColorPalette via FNV-1a.
func ResolveChatterColorHex(platform, username, rawHex string) string {
	if norm := normalizeHexColor(rawHex); norm != "" {
		return ensureReadableOnDark(norm)
	}
	key := strings.ToLower(strings.TrimSpace(platform)) + ":" + strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	idx := int(h.Sum32()) % len(DefaultChatterColorPalette)
	return DefaultChatterColorPalette[idx]
}

func normalizeHexColor(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return ""
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return ""
		}
	}
	return "#" + strings.ToUpper(s)
}

func ensureReadableOnDark(hexColor string) string {
	raw := strings.TrimPrefix(hexColor, "#")
	if len(raw) != 6 {
		return hexColor
	}
	r64, _ := strconv.ParseUint(raw[0:2], 16, 8)
	g64, _ := strconv.ParseUint(raw[2:4], 16, 8)
	b64, _ := strconv.ParseUint(raw[4:6], 16, 8)
	r, g, b := float64(r64), float64(g64), float64(b64)
	// Relative perceived luminance
	lum := 0.299*r + 0.587*g + 0.114*b
	if lum >= 105 {
		return hexColor
	}
	// Blend toward white so dark blues/maroons/blacks remain legible on #0F172A
	boost := (115 - lum) / 255.0
	if boost < 0.35 {
		boost = 0.35
	}
	nr := uint8(min(255, r+(255-r)*boost))
	ng := uint8(min(255, g+(255-g)*boost))
	nb := uint8(min(255, b+(255-b)*boost))
	return fmt.Sprintf("#%02X%02X%02X", nr, ng, nb)
}

// SelectChatterVoice resolves the TTS voice ID for a chatter on a given stream slot.
func SelectChatterVoice(ev chat.Event, slot config.StreamSlotConfig, cfg *config.Config, availableVoices []string) string {
	return SelectChatterVoiceWithPronouns(ev, slot, cfg, availableVoices, nil)
}

// SelectChatterVoiceWithPronouns resolves the TTS voice ID for a chatter using explicit overrides,
// smart gender detection (pronoun cache + username analysis), and deterministic hashing without mutating ChatterVoices.
func SelectChatterVoiceWithPronouns(ev chat.Event, slot config.StreamSlotConfig, cfg *config.Config, availableVoices []string, pronouns *chat.PronounCache) string {
	userKey := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ev.UserName), "@"))
	if userKey == "" {
		userKey = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ev.DisplayName), "@"))
	}
	platLower := strings.ToLower(strings.TrimSpace(ev.Platform))

	if cfg != nil && len(cfg.Identity.ChatterVoices) > 0 && userKey != "" {
		if v, ok := cfg.Identity.ChatterVoices[platLower+":"+userKey]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if v, ok := cfg.Identity.ChatterVoices[userKey]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}

	fallbackVoice := strings.TrimSpace(slot.Voice)
	if fallbackVoice == "" && cfg != nil && strings.TrimSpace(cfg.Audio.DefaultVoice) != "" {
		fallbackVoice = strings.TrimSpace(cfg.Audio.DefaultVoice)
	}
	if fallbackVoice == "" {
		fallbackVoice = "supertonic:f1"
	}

	if cfg != nil && cfg.Audio.HashChatterVoices && userKey != "" {
		pool := slot.VoicePool
		if len(pool) == 0 {
			pool = availableVoices
		}
		if len(pool) == 0 {
			pool = tts.VoiceIDsForModels(cfg.Audio.EffectiveEnabledModels())
		}
		if len(pool) == 0 {
			pool = DefaultSupertonicVoicePool
		}
		return chat.SelectSmartChatterVoice(
			platLower,
			userKey,
			ev.DisplayName,
			pool,
			fallbackVoice,
			cfg.Audio.SmartGenderVoices,
			cfg.Audio.UnknownGenderPolicy,
			pronouns,
		)
	}

	return fallbackVoice
}

// FormatSpokenText constructs the final text sent to the TTS engine, including the smart
// channel-switch prefix (only spoken when switching active channels while watching >1 stream)
// and optional chatter name prefix.
func FormatSpokenText(
	ev chat.Event,
	cleanedMsg string,
	slot config.StreamSlotConfig,
	cfg *config.Config,
	activeSlotCount int,
	lastSpokenSlotID string,
) (string, string) {
	spoken, nextSlot, _, _ := FormatSpokenTextWithCoalesce(ev, cleanedMsg, slot, cfg, activeSlotCount, lastSpokenSlotID, "", time.Time{}, time.Now())
	return spoken, nextSlot
}

// FormatSpokenTextWithCoalesce constructs the spoken TTS text while applying both smart channel-switch
// announcements and consecutive-speaker coalescing within cfg.Audio.CoalesceSeconds.
func FormatSpokenTextWithCoalesce(
	ev chat.Event,
	cleanedMsg string,
	slot config.StreamSlotConfig,
	cfg *config.Config,
	activeSlotCount int,
	lastSpokenSlotID string,
	lastSpeakerKey string,
	lastSpokenAt time.Time,
	now time.Time,
) (spoken string, nextSlotID string, nextSpeakerKey string, coalesced bool) {
	slotID := slot.SlotID()
	var b strings.Builder

	channelSwitched := slotID != lastSpokenSlotID
	if cfg != nil && cfg.Audio.SpeakChannelOnSwitch && activeSlotCount > 1 && channelSwitched {
		b.WriteString(slot.EffectiveLabel())
		b.WriteString(", ")
	}

	speaker := strings.TrimLeft(chatterName(ev), "@")
	if speaker != "" && cfg != nil && cfg.Audio.NaturalizeNames {
		speaker = chat.NaturalizeUsername(speaker)
	}
	spKey := speakerKey(ev)

	if cfg != nil && cfg.Audio.SpeakNames && cfg.Audio.CoalesceSeconds > 0 &&
		(ev.Kind == "" || ev.Kind == chat.EventKindMessage) &&
		!channelSwitched && spKey != "" && spKey == lastSpeakerKey && !lastSpokenAt.IsZero() {
		if now.Sub(lastSpokenAt) <= time.Duration(cfg.Audio.CoalesceSeconds)*time.Second {
			coalesced = true
		}
	}

	switch ev.Kind {
	case chat.EventKindSub, chat.EventKindResub:
		if strings.TrimSpace(cleanedMsg) != "" {
			fmt.Fprintf(&b, "%s subscribed: %s", speaker, cleanedMsg)
		} else {
			fmt.Fprintf(&b, "%s subscribed!", speaker)
		}
	case chat.EventKindCheer, chat.EventKindSuperChat, chat.EventKindCharityDonation:
		if strings.TrimSpace(cleanedMsg) != "" {
			fmt.Fprintf(&b, "%s cheered: %s", speaker, cleanedMsg)
		} else {
			fmt.Fprintf(&b, "%s cheered!", speaker)
		}
	case chat.EventKindRaid:
		fmt.Fprintf(&b, "%s raided the stream!", speaker)
	case chat.EventKindFollow:
		fmt.Fprintf(&b, "%s followed!", speaker)
	default:
		if cfg != nil && cfg.Audio.SpeakNames && speaker != "" && !coalesced {
			fmt.Fprintf(&b, "%s: %s", speaker, cleanedMsg)
		} else {
			b.WriteString(cleanedMsg)
		}
	}

	return strings.TrimSpace(b.String()), slotID, spKey, coalesced
}
