package listener

import (
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/TownCrier/pkg/chat"
)

// commonHypeSpamTokens contains lowercase single-word hype tokens skipped when SkipShortSpam is enabled.
var commonHypeSpamTokens = map[string]struct{}{
	"lul": {}, "lol": {}, "lmao": {}, "lmfao": {}, "rofl": {},
	"kekw": {}, "omegalul": {}, "lulw": {}, "pog": {}, "poggers": {},
	"pogchamp": {}, "w": {}, "l": {}, "f": {}, "gg": {}, "ggs": {},
	"ez": {}, "rip": {}, "ype": {}, "hype": {}, "catjam": {},
	"pepega": {}, "sadge": {}, "copium": {}, "monkas": {}, "monkaw": {},
	"777": {}, "o7": {}, "<3": {}, "kappa": {}, "based": {}, "real": {},
}

// CrowdEvalResult describes the outcome of evaluating a chat event against Crowd Control rules.
type CrowdEvalResult struct {
	Allow      bool
	Reason     string
	IsMention  bool
	IsPriority bool
}

// CrowdController enforces role filters, hype-spam filtering, per-chatter cooldowns,
// global pacing, and priority/mention bypasses across active stream slots.
type CrowdController struct {
	mu                  sync.Mutex
	lastSpokenByChatter map[string]time.Time
	lastSpokenGlobal    time.Time
}

// NewCrowdController creates a new CrowdController instance.
func NewCrowdController() *CrowdController {
	return &CrowdController{
		lastSpokenByChatter: make(map[string]time.Time),
	}
}

// Evaluate checks whether an inbound event should be spoken or skipped under the slot & global rules.
func (cc *CrowdController) Evaluate(ev chat.Event, cleanedText string, slot config.StreamSlotConfig, cfg *config.Config, now time.Time) CrowdEvalResult {
	if slot.Muted {
		return CrowdEvalResult{Allow: false, Reason: "slot_muted"}
	}

	userLower := strings.ToLower(strings.TrimSpace(ev.UserName))
	if userLower == "" {
		userLower = strings.ToLower(strings.TrimSpace(ev.DisplayName))
	}

	// 1. Check ignored/muted chatters first
	if userLower != "" && containsFold(cfg.Identity.IgnoredChatters, userLower) {
		return CrowdEvalResult{Allow: false, Reason: "muted_chatter"}
	}

	// 2. Check non-chat events without text (follows, shares, gift streaks)
	trimmedText := strings.TrimSpace(cleanedText)
	speakNoText := cfg.Crowd.SpeakNoTextEvents
	if slot.SpeakNoTextEvents != nil {
		speakNoText = *slot.SpeakNoTextEvents
	}

	if ev.Kind != "" && ev.Kind != chat.EventKindMessage {
		isNoTextEvent := ev.Kind == chat.EventKindFollow ||
			ev.Kind == chat.EventKindShare ||
			ev.Kind == chat.EventKindGift ||
			ev.Kind == chat.EventKindGiftSub ||
			trimmedText == ""
		if isNoTextEvent && !speakNoText {
			return CrowdEvalResult{Allow: false, Reason: "no_text_event"}
		}
	}

	if trimmedText == "" && (ev.Kind == "" || ev.Kind == chat.EventKindMessage) {
		return CrowdEvalResult{Allow: false, Reason: "empty_message"}
	}

	// 3. Detect @mention or reply to viewer's configured username(s)
	isMention := matchesMyUsernames(ev, cfg.Identity.MyUsernames)
	isBroadcaster := hasRoleFold(ev.Roles, "broadcaster", "owner", "streamer") ||
		strings.EqualFold(userLower, strings.TrimPrefix(strings.ToLower(slot.Channel), "@"))
	isPriorityFriend := userLower != "" && containsFold(cfg.Identity.PriorityChatters, userLower)
	isPaidOrMajorTextEvent := (ev.Kind == chat.EventKindCheer ||
		ev.Kind == chat.EventKindSub ||
		ev.Kind == chat.EventKindResub ||
		ev.Kind == chat.EventKindSuperChat ||
		ev.Kind == chat.EventKindCharityDonation ||
		ev.Kind == chat.EventKindRaid ||
		ev.Amount > 0) && (trimmedText != "" || ev.Kind == chat.EventKindRaid)

	// Priority bypass: broadcaster, priority friends, @mentions, and paid/major text events bypass crowd gates
	if isMention || isBroadcaster || isPriorityFriend || isPaidOrMajorTextEvent {
		cc.recordSpoken(slot.SlotID(), userLower, now)
		return CrowdEvalResult{
			Allow:      true,
			Reason:     "priority_bypass",
			IsMention:  isMention,
			IsPriority: true,
		}
	}

	// 4. Role / Badge gate
	mode := strings.TrimSpace(slot.CrowdFilter)
	if mode == "" {
		mode = cfg.Crowd.DefaultFilterMode
	}
	isMod := hasRoleFold(ev.Roles, "moderator", "mod")
	isVIP := hasRoleFold(ev.Roles, "vip")
	isSub := hasRoleFold(ev.Roles, "subscriber", "sub", "member", "founder")

	switch mode {
	case config.FilterSubsVIPsMods:
		if !isMod && !isVIP && !isSub {
			return CrowdEvalResult{Allow: false, Reason: "role_filter_subs_vips_mods"}
		}
	case config.FilterModsStreamerMentions:
		if !isMod {
			return CrowdEvalResult{Allow: false, Reason: "role_filter_mods_mentions"}
		}
	}

	// 5. Minimum quality / short hype-spam gate
	if cfg.Crowd.SkipShortSpam && IsShortHypeSpam(trimmedText) {
		return CrowdEvalResult{Allow: false, Reason: "short_hype_spam"}
	}

	// 6. Per-chatter cooldown & global pacing
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cfg.Crowd.GlobalPacingMs > 0 && !cc.lastSpokenGlobal.IsZero() {
		if now.Sub(cc.lastSpokenGlobal) < time.Duration(cfg.Crowd.GlobalPacingMs)*time.Millisecond {
			return CrowdEvalResult{Allow: false, Reason: "global_pacing"}
		}
	}

	if cfg.Crowd.ChatterCooldownSec > 0 && userLower != "" {
		key := slot.SlotID() + ":" + userLower
		if last, ok := cc.lastSpokenByChatter[key]; ok {
			if now.Sub(last) < time.Duration(cfg.Crowd.ChatterCooldownSec)*time.Second {
				return CrowdEvalResult{Allow: false, Reason: "chatter_cooldown"}
			}
		}
		cc.lastSpokenByChatter[key] = now
	}

	cc.lastSpokenGlobal = now
	return CrowdEvalResult{
		Allow:  true,
		Reason: "allowed",
	}
}

func (cc *CrowdController) recordSpoken(slotID, userLower string, now time.Time) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if userLower != "" {
		cc.lastSpokenByChatter[slotID+":"+userLower] = now
	}
	cc.lastSpokenGlobal = now
}

func hasRoleFold(roles []string, targets ...string) bool {
	for _, r := range roles {
		for _, t := range targets {
			if strings.EqualFold(strings.TrimSpace(r), t) {
				return true
			}
		}
	}
	return false
}

// IsShortHypeSpam returns true if the cleaned text is a 1-word hype token (e.g. "LUL", "GG", "W", "F")
// or contains no letters/digits or is a single token of <= 2 characters.
func IsShortHypeSpam(text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return true
	}

	hasAlphaNum := false
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			hasAlphaNum = true
			break
		}
	}
	if !hasAlphaNum {
		return true
	}

	if len(fields) == 1 {
		token := strings.ToLower(strings.Trim(fields[0], ".,!?~*_-:;\"'()[]{}"))
		if len([]rune(token)) <= 2 {
			return true
		}
		if _, isHype := commonHypeSpamTokens[token]; isHype {
			return true
		}
	}

	if len(fields) <= 4 {
		first := strings.ToLower(strings.Trim(fields[0], ".,!?~*_-:;\"'()[]{}"))
		if _, isHype := commonHypeSpamTokens[first]; isHype {
			allSame := true
			for _, f := range fields[1:] {
				clean := strings.ToLower(strings.Trim(f, ".,!?~*_-:;\"'()[]{}"))
				if clean != first {
					allSame = false
					break
				}
			}
			if allSame {
				return true
			}
		}
	}

	return false
}

func matchesMyUsernames(ev chat.Event, myUsernames []string) bool {
	if len(myUsernames) == 0 {
		return false
	}
	lowerMsg := strings.ToLower(ev.Text)
	for _, rawName := range myUsernames {
		name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(rawName), "@"))
		if name == "" {
			continue
		}
		if ev.IsReply && strings.EqualFold(strings.TrimPrefix(ev.ReplyToUser, "@"), name) {
			return true
		}
		if containsFold(ev.Mentions, name) {
			return true
		}
		if lowerMsg != "" && (strings.Contains(lowerMsg, "@"+name) || containsWordBoundary(lowerMsg, name)) {
			return true
		}
	}
	return false
}

func containsWordBoundary(haystack, needle string) bool {
	idx := strings.Index(haystack, needle)
	for idx != -1 {
		beforeOK := idx == 0 || !isWordChar(rune(haystack[idx-1]))
		end := idx + len(needle)
		afterOK := end == len(haystack) || !isWordChar(rune(haystack[end]))
		if beforeOK && afterOK {
			return true
		}
		next := strings.Index(haystack[end:], needle)
		if next == -1 {
			break
		}
		idx = end + next
	}
	return false
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
}

func containsFold(list []string, target string) bool {
	t := strings.TrimPrefix(strings.TrimSpace(target), "@")
	for _, item := range list {
		clean := strings.TrimPrefix(strings.TrimSpace(item), "@")
		if strings.EqualFold(clean, t) {
			return true
		}
	}
	return false
}
