package listener

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/TownCrier/pkg/audio"
	"github.com/mrpoundsign/TownCrier/pkg/chat"
)

func TestParseTargetInput(t *testing.T) {
	cases := []struct {
		raw      string
		fallback string
		wantPlat string
		wantChan string
	}{
		{"https://www.twitch.tv/mocomade", "twitch", "twitch", "mocomade"},
		{"twitch.tv/cohhcarnage/chat", "youtube", "twitch", "cohhcarnage"},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "twitch", "youtube", "dQw4w9WgXcQ"},
		{"https://youtu.be/abc123XYZ", "twitch", "youtube", "abc123XYZ"},
		{"https://www.youtube.com/@MoCoMade/live", "twitch", "youtube", "@MoCoMade"},
		{"https://kick.com/xqc", "twitch", "kick", "xqc"},
		{"https://www.tiktok.com/@creator_handle/live", "twitch", "tiktok", "creator_handle"},
		{"yt:@SomeCreator", "twitch", "youtube", "@SomeCreator"},
		{"kick:somekickstreamer", "twitch", "kick", "somekickstreamer"},
		{"tiktok:@ttstreamer", "twitch", "tiktok", "ttstreamer"},
		{"barechannel", "twitch", "twitch", "barechannel"},
	}

	for _, tc := range cases {
		plat, ch, err := ParseTargetInput(tc.raw, tc.fallback)
		if err != nil {
			t.Fatalf("ParseTargetInput(%q) unexpected error: %v", tc.raw, err)
		}
		if plat != tc.wantPlat || ch != tc.wantChan {
			t.Errorf("ParseTargetInput(%q) = (%q, %q), want (%q, %q)", tc.raw, plat, ch, tc.wantPlat, tc.wantChan)
		}
	}
}

func TestCrowdControllerFilteringAndPriorityBypass(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Identity.MyUsernames = []string{"BlindViewer", "mrpoundsign", "youtube:@MyYTHandle"}
	cfg.Identity.PriorityChatters = []string{"BestFriend"}
	cfg.Identity.IgnoredChatters = []string{"ToxicUser"}
	cfg.Crowd.ChatterCooldownSec = 10
	cfg.Crowd.SkipShortSpam = true

	slot := config.StreamSlotConfig{
		Platform:    "twitch",
		Channel:     "streamer_one",
		Volume:      1.0,
		CrowdFilter: config.FilterSubsVIPsMods,
	}

	cc := NewCrowdController()
	now := time.Now()

	// 0. Listener's own messages (matching MyUsernames case-insensitively or with @) are skipped (Reason="own_message")
	resOwn := cc.Evaluate(chat.Event{
		Platform:    "twitch",
		Kind:        chat.EventKindMessage,
		UserName:    "MrPoundsign",
		DisplayName: "MrPoundsign",
		Roles:       []string{"subscriber"},
		Text:        "I am typing in chat myself!",
	}, "I am typing in chat myself!", slot, cfg, now)
	if resOwn.Allow || resOwn.Reason != "own_message" {
		t.Fatalf("expected own_message rejection for MrPoundsign, got %+v", resOwn)
	}

	// Platform-scoped username (youtube:@MyYTHandle) should NOT trigger @mention on Twitch
	resOtherPlatMention := cc.Evaluate(chat.Event{
		Platform: "twitch",
		Kind:     chat.EventKindMessage,
		UserName: "RandomViewer",
		Text:     "Hey @MyYTHandle are you here?",
	}, "Hey @MyYTHandle are you here?", slot, cfg, now)
	if resOtherPlatMention.IsMention || resOtherPlatMention.Allow {
		t.Fatalf("expected youtube-scoped @MyYTHandle mention to be ignored on twitch, got %+v", resOtherPlatMention)
	}

	// 1. Muted chatter is dropped
	res := cc.Evaluate(chat.Event{
		Kind:     chat.EventKindMessage,
		UserName: "ToxicUser",
		Roles:    []string{"subscriber"},
		Text:     "Hello everyone in chat!",
	}, "Hello everyone in chat!", slot, cfg, now)
	if res.Allow || res.Reason != "muted_chatter" {
		t.Fatalf("expected muted_chatter rejection, got %+v", res)
	}

	// 2. Non-sub normal viewer is rejected by FilterSubsVIPsMods
	res = cc.Evaluate(chat.Event{
		Kind:     chat.EventKindMessage,
		UserName: "RandomViewer",
		Text:     "This is a full sentence from a non-sub",
	}, "This is a full sentence from a non-sub", slot, cfg, now)
	if res.Allow || res.Reason != "role_filter_subs_vips_mods" {
		t.Fatalf("expected role_filter_subs_vips_mods rejection, got %+v", res)
	}

	// 3. Mention of MyUsernames by a non-sub BYPASSES role filter and marks IsMention=true
	res = cc.Evaluate(chat.Event{
		Kind:     chat.EventKindMessage,
		UserName: "RandomViewer",
		Text:     "Hey @BlindViewer what do you think of this boss fight?",
	}, "Hey @BlindViewer what do you think of this boss fight?", slot, cfg, now)
	if !res.Allow || !res.IsMention || !res.IsPriority {
		t.Fatalf("expected @mention priority bypass, got %+v", res)
	}

	// 4. Priority friend bypasses role filter
	res = cc.Evaluate(chat.Event{
		Kind:     chat.EventKindMessage,
		UserName: "BestFriend",
		Text:     "LUL", // Even short message from priority friend bypasses spam filter
	}, "LUL", slot, cfg, now)
	if !res.Allow || !res.IsPriority {
		t.Fatalf("expected priority friend bypass, got %+v", res)
	}

	// 5. Subscriber sending hype spam "LUL LUL" is dropped by SkipShortSpam
	res = cc.Evaluate(chat.Event{
		Kind:     chat.EventKindMessage,
		UserName: "SubChatter",
		Roles:    []string{"subscriber"},
		Text:     "LUL LUL",
	}, "LUL LUL", slot, cfg, now)
	if res.Allow || res.Reason != "short_hype_spam" {
		t.Fatalf("expected short_hype_spam rejection, got %+v", res)
	}

	// 6. Subscriber sending real sentence is allowed once, then rate-limited by ChatterCooldownSec
	res = cc.Evaluate(chat.Event{
		Kind:     chat.EventKindMessage,
		UserName: "SubChatter",
		Roles:    []string{"subscriber"},
		Text:     "That dodge timing was incredible!",
	}, "That dodge timing was incredible!", slot, cfg, now)
	if !res.Allow {
		t.Fatalf("expected subscriber message to be allowed, got %+v", res)
	}

	res2 := cc.Evaluate(chat.Event{
		Kind:     chat.EventKindMessage,
		UserName: "SubChatter",
		Roles:    []string{"subscriber"},
		Text:     "Here is a second message two seconds later",
	}, "Here is a second message two seconds later", slot, cfg, now.Add(2*time.Second))
	if res2.Allow || res2.Reason != "chatter_cooldown" {
		t.Fatalf("expected chatter_cooldown rejection, got %+v", res2)
	}

	// 7. Follow event (no text) is muted by default
	resFollow := cc.Evaluate(chat.Event{
		Kind:     chat.EventKindFollow,
		UserName: "NewFollower",
	}, "", slot, cfg, now.Add(20*time.Second))
	if resFollow.Allow || resFollow.Reason != "no_text_event" {
		t.Fatalf("expected no_text_event rejection for follow, got %+v", resFollow)
	}
}

func TestSelectChatterVoiceAndSmartChannelSwitch(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Audio.HashChatterVoices = true
	cfg.Audio.SpeakChannelOnSwitch = true
	cfg.Audio.SpeakNames = true
	cfg.Identity.ChatterVoices["specialuser"] = "supertonic:m5"

	slotA := config.StreamSlotConfig{
		Platform:    "twitch",
		Channel:     "streamer_a",
		DisplayName: "Streamer A",
	}
	slotB := config.StreamSlotConfig{
		Platform:    "twitch",
		Channel:     "streamer_b",
		DisplayName: "Streamer B",
	}

	ev1 := chat.Event{Platform: "twitch", UserName: "Alice", DisplayName: "Alice", Text: "Hello there"}
	v1 := SelectChatterVoice(ev1, slotA, cfg, DefaultSupertonicVoicePool)
	v2 := SelectChatterVoice(ev1, slotA, cfg, DefaultSupertonicVoicePool)
	if v1 == "" || v1 != v2 {
		t.Fatalf("expected deterministic voice hash for Alice, got %q and %q", v1, v2)
	}
	if chat.ClassifyVoiceGender(v1) != chat.GenderFeminine {
		t.Fatalf("expected feminine voice for Alice with SmartGenderVoices=true, got %q", v1)
	}

	evMasc := chat.Event{Platform: "twitch", UserName: "mrpoundsign", DisplayName: "MrPoundSign", Text: "Testing"}
	vMasc := SelectChatterVoice(evMasc, slotA, cfg, DefaultSupertonicVoicePool)
	if chat.ClassifyVoiceGender(vMasc) != chat.GenderMasculine {
		t.Fatalf("expected masculine voice for MrPoundSign with SmartGenderVoices=true, got %q", vMasc)
	}
	if len(cfg.Identity.ChatterVoices) != 1 {
		t.Fatalf("expected ChatterVoices map not to be mutated by auto-hashing, got %d entries", len(cfg.Identity.ChatterVoices))
	}

	evOverride := chat.Event{Platform: "twitch", UserName: "SpecialUser", Text: "Custom voice"}
	if got := SelectChatterVoice(evOverride, slotA, cfg, DefaultSupertonicVoicePool); got != "supertonic:m5" {
		t.Fatalf("expected explicit chatter voice supertonic:m5, got %q", got)
	}

	// Smart channel-switch prefix when watching 2 streams:
	spoken1, lastSlot := FormatSpokenText(ev1, "First message", slotA, cfg, 2, "")
	if !strings.HasPrefix(spoken1, "Streamer A, Alice: First message") {
		t.Fatalf("expected channel prefix on first message, got %q", spoken1)
	}

	// Second consecutive message from the SAME channel should NOT repeat "Streamer A, "
	spoken2, lastSlot := FormatSpokenText(ev1, "Second message", slotA, cfg, 2, lastSlot)
	if strings.HasPrefix(spoken2, "Streamer A,") || spoken2 != "Alice: Second message" {
		t.Fatalf("expected no channel prefix on consecutive message from same channel, got %q", spoken2)
	}

	// Message from Streamer B SHOULD announce "Streamer B, "
	spoken3, _ := FormatSpokenText(ev1, "Over on B", slotB, cfg, 2, lastSlot)
	if !strings.HasPrefix(spoken3, "Streamer B, Alice: Over on B") {
		t.Fatalf("expected channel prefix when switching to Streamer B, got %q", spoken3)
	}

	// Consecutive speaker coalescing within CoalesceSeconds (30s):
	now := time.Now()
	cfg.Audio.CoalesceSeconds = 30
	spFirst, slotAfter1, spkAfter1, coal1 := FormatSpokenTextWithCoalesce(ev1, "Part one", slotA, cfg, 1, "", "", time.Time{}, now)
	if coal1 || spFirst != "Alice: Part one" {
		t.Fatalf("expected non-coalesced first message 'Alice: Part one', got %q (coal=%v)", spFirst, coal1)
	}
	spSecond, _, _, coal2 := FormatSpokenTextWithCoalesce(ev1, "and part two", slotA, cfg, 1, slotAfter1, spkAfter1, now, now.Add(5*time.Second))
	if !coal2 || spSecond != "and part two" {
		t.Fatalf("expected coalesced follow-up 'and part two' without repeating username, got %q (coal=%v)", spSecond, coal2)
	}

	// Username naturalizer enabled by default, can be toggled off
	evRawHandle := chat.Event{Platform: "twitch", UserName: "xx_darkknight99_xx", DisplayName: "xX_DarkKnight99_Xx", Text: "Good game"}
	spNat, _ := FormatSpokenText(evRawHandle, "Good game", slotA, cfg, 1, "")
	if spNat != "Dark Knight 99: Good game" {
		t.Fatalf("expected naturalized speaker 'Dark Knight 99: Good game', got %q", spNat)
	}
	cfg.Audio.NaturalizeNames = false
	spRaw, _ := FormatSpokenText(evRawHandle, "Good game", slotA, cfg, 1, "")
	if spRaw != "xX_DarkKnight99_Xx: Good game" {
		t.Fatalf("expected raw speaker 'xX_DarkKnight99_Xx: Good game' when NaturalizeNames=false, got %q", spRaw)
	}
	cfg.Audio.NaturalizeNames = true

	// User chat color extraction & dark-theme readability clamping
	kickRaw := map[string]any{
		"sender": map[string]any{
			"identity": map[string]any{
				"color": "#53FC18",
			},
		},
	}
	if gotCol := ExtractUserColor(chat.Event{Platform: "kick", Raw: kickRaw}); gotCol != "#53FC18" {
		t.Fatalf("expected extracted Kick color #53FC18, got %q", gotCol)
	}
	if clamped := ResolveChatterColorHex("twitch", "Alice", "#000080"); clamped == "#000080" || clamped == "" {
		t.Fatalf("expected dark navy #000080 to be lightness-clamped for dark theme, got %q", clamped)
	}
}

type mockConnector struct {
	startedTarget string
	stopped       bool
	onStatus      func(status string, detail string)
}

func (m *mockConnector) Start(target string) error {
	m.startedTarget = target
	if m.onStatus != nil {
		m.onStatus("connected", "Live")
	}
	return nil
}

func (m *mockConnector) Stop() error {
	m.stopped = true
	return nil
}

type mockPlayer struct {
	mu      sync.Mutex
	plays   []audio.PlayOptions
	aborted bool
}

func (p *mockPlayer) Play(samples []float32, sampleRate int) error {
	return p.PlayWithOptions(samples, sampleRate, audio.PlayOptions{Gain: 1.0, Pan: 0.0})
}

func (p *mockPlayer) PlayWithOptions(_ []float32, _ int, opts audio.PlayOptions) error {
	p.mu.Lock()
	p.plays = append(p.plays, opts)
	p.mu.Unlock()
	return nil
}

func (p *mockPlayer) Abort() {
	p.mu.Lock()
	p.aborted = true
	p.mu.Unlock()
}

func (p *mockPlayer) Stop()                                   { p.Abort() }
func (p *mockPlayer) PanicStop()                              { p.Abort() }
func (p *mockPlayer) ResetPanic()                             {}
func (p *mockPlayer) SwitchDevice(_ string) error             { return nil }
func (p *mockPlayer) SetVolume(_ float32)                     {}
func (p *mockPlayer) Volume() float32                         { return 1.0 }
func (p *mockPlayer) IsPlaying() bool                         { return false }
func (p *mockPlayer) Close() error                            { return nil }
func (p *mockPlayer) SetFallbackListener(_ func(reason string)) {}

type mockTTSEngine struct{}

func (e *mockTTSEngine) Synthesize(_ string, _ string, _ float32) ([]float32, int, error) {
	return []float32{0.1, -0.1, 0.2, -0.2}, 24000, nil
}
func (e *mockTTSEngine) Voices() []string { return DefaultSupertonicVoicePool }
func (e *mockTTSEngine) NumSpeakers() int { return len(DefaultSupertonicVoicePool) }
func (e *mockTTSEngine) Close() error     { return nil }

func TestMultiStreamSessionEndToEnd(t *testing.T) {
	store := config.NewMemoryStore()
	cfg, _ := store.Load()
	cfg.Identity.MyUsernames = []string{"ViewerHero"}

	sess := NewSession(cfg, store)
	defer func() { _ = sess.Close() }()

	sess.SetConnectorFactory(func(_ string, _ func(ev *chat.Event), onStatus func(status string, detail string)) (SlotConnector, error) {
		return &mockConnector{onStatus: onStatus}, nil
	})

	player := &mockPlayer{}
	sess.SetPlayer(player)
	sess.SetEngine(&mockTTSEngine{})

	// Add 2 simultaneous Twitch co-streams panned Left (-0.8) and Right (+0.8)
	if err := sess.AddSlot(config.StreamSlotConfig{
		Platform:    "twitch",
		Channel:     "costreamer_left",
		DisplayName: "LeftStream",
		Volume:      0.9,
		Pan:         -0.8,
	}); err != nil {
		t.Fatalf("AddSlot left failed: %v", err)
	}
	if err := sess.AddSlot(config.StreamSlotConfig{
		Platform:    "twitch",
		Channel:     "costreamer_right",
		DisplayName: "RightStream",
		Volume:      1.1,
		Pan:         0.8,
	}); err != nil {
		t.Fatalf("AddSlot right failed: %v", err)
	}

	// Send a message on LeftStream and a @mention on RightStream
	sess.HandleInboundEvent("twitch:costreamer_left", &chat.Event{
		Kind:     chat.EventKindMessage,
		UserName: "ChatterOne",
		Text:     "Great teamwork on the left flank!",
	})
	sess.HandleInboundEvent("twitch:costreamer_right", &chat.Event{
		Kind:     chat.EventKindMessage,
		UserName: "ChatterTwo",
		Text:     "Hey @ViewerHero watch out on the right!",
	})

	// Wait for speech worker to finish speaking both entries
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		logs := sess.LogSnapshot()
		if len(logs) >= 2 && logs[0].Status == StatusSpoken && logs[1].Status == StatusSpoken {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	logs := sess.LogSnapshot()
	if len(logs) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(logs))
	}
	if logs[0].Status != StatusSpoken || logs[1].Status != StatusSpoken {
		t.Fatalf("expected both entries spoken, got %s and %s", logs[0].Status, logs[1].Status)
	}
	if !logs[1].IsMention {
		t.Fatalf("expected second entry to be marked IsMention=true")
	}

	player.mu.Lock()
	playCount := len(player.plays)
	player.mu.Unlock()
	if playCount < 3 {
		// At least: Left TTS + Right Mention Earcon + Right TTS
		t.Fatalf("expected at least 3 PlayWithOptions calls (including mention earcon), got %d", playCount)
	}

	// Save active slots as a Lineup and verify persistence
	lineup, err := sess.SaveCurrentAsLineup("Co-Stream Setup")
	if err != nil {
		t.Fatalf("SaveCurrentAsLineup failed: %v", err)
	}
	if len(lineup.Slots) != 2 {
		t.Fatalf("expected 2 slots in saved lineup, got %d", len(lineup.Slots))
	}
}
