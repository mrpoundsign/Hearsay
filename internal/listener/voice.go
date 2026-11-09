package listener

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/TownCrier/pkg/chat"
)

// DefaultSupertonicVoicePool is the fallback voice pool when deterministic chatter voice hashing is active.
var DefaultSupertonicVoicePool = []string{
	"supertonic:f1", "supertonic:f2", "supertonic:f3", "supertonic:f4", "supertonic:f5",
	"supertonic:m1", "supertonic:m2", "supertonic:m3", "supertonic:m4", "supertonic:m5",
}

func chatterName(ev chat.Event) string {
	if strings.TrimSpace(ev.DisplayName) != "" {
		return strings.TrimSpace(ev.DisplayName)
	}
	return strings.TrimSpace(ev.UserName)
}

// SelectChatterVoice resolves the TTS voice ID for a chatter on a given stream slot.
func SelectChatterVoice(ev chat.Event, slot config.StreamSlotConfig, cfg *config.Config, availableVoices []string) string {
	userKey := strings.ToLower(strings.TrimSpace(ev.UserName))
	if userKey == "" {
		userKey = strings.ToLower(strings.TrimSpace(ev.DisplayName))
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

	if cfg != nil && cfg.Audio.HashChatterVoices && userKey != "" {
		pool := slot.VoicePool
		if len(pool) == 0 {
			pool = availableVoices
		}
		if len(pool) == 0 {
			pool = DefaultSupertonicVoicePool
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(platLower + ":" + userKey))
		idx := int(h.Sum32()) % len(pool)
		return pool[idx]
	}

	if strings.TrimSpace(slot.Voice) != "" {
		return strings.TrimSpace(slot.Voice)
	}
	if cfg != nil && strings.TrimSpace(cfg.Audio.DefaultVoice) != "" {
		return strings.TrimSpace(cfg.Audio.DefaultVoice)
	}
	return "supertonic:f1"
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
	slotID := slot.SlotID()
	var b strings.Builder

	if cfg != nil && cfg.Audio.SpeakChannelOnSwitch && activeSlotCount > 1 && slotID != lastSpokenSlotID {
		b.WriteString(slot.EffectiveLabel())
		b.WriteString(", ")
	}

	speaker := chatterName(ev)
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
		if cfg != nil && cfg.Audio.SpeakNames && speaker != "" {
			fmt.Fprintf(&b, "%s: %s", speaker, cleanedMsg)
		} else {
			b.WriteString(cleanedMsg)
		}
	}

	return strings.TrimSpace(b.String()), slotID
}
