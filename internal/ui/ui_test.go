package ui

import (
	"testing"

	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/Hearsay/internal/listener"
	"github.com/mrpoundsign/TownCrier/pkg/audio"
	"github.com/mrpoundsign/TownCrier/pkg/chat"
)

type noopConnector struct {
	onStatus func(status string, detail string)
}

func (n *noopConnector) Start(_ string) error {
	if n.onStatus != nil {
		n.onStatus("connected", "Live")
	}
	return nil
}
func (n *noopConnector) Stop() error { return nil }

func TestMainWindowQuickTuneMiniPlayerAndTheme(t *testing.T) {
	a := fynetest.NewTempApp(t)
	store := config.NewMemoryStore()
	cfg, _ := store.Load()

	sess := listener.NewSession(cfg, store)
	defer func() { _ = sess.Close() }()

	sess.SetConnectorFactory(func(_ string, _ func(ev *chat.Event), onStatus func(status string, detail string)) (listener.SlotConnector, error) {
		return &noopConnector{onStatus: onStatus}, nil
	})

	mockEnum := func() ([]audio.DeviceInfo, error) {
		return []audio.DeviceInfo{
			{ID: "dev-1", Name: "Headphones (Stereo)", IsDefault: true},
		}, nil
	}

	mw := NewMainWindow(a, sess, mockEnum)

	// Verify Quick Tune adds a stream slot
	mw.QuickTuneEntry.SetText("https://www.twitch.tv/mocomade")
	fynetest.Tap(mw.TuneInBtn)

	slots := sess.SlotsSnapshot()
	if len(slots) != 1 || slots[0].Config.Channel != "mocomade" {
		t.Fatalf("expected 1 tuned slot for mocomade, got %+v", slots)
	}

	// Verify Platform Selector + Channel Name without platform:channel syntax
	mw.PlatformSelect.SetSelected("YouTube")
	mw.QuickTuneEntry.SetText("@MoCoMade")
	fynetest.Tap(mw.TuneInBtn)

	slots = sess.SlotsSnapshot()
	if len(slots) != 2 || slots[1].Config.Platform != "youtube" || slots[1].Config.Channel != "@MoCoMade" {
		t.Fatalf("expected 2nd tuned slot for youtube/@MoCoMade, got %+v", slots)
	}

	// Verify Channel Avatar Icon generation and toggle
	res := ChannelAvatarResource("twitch", "mocomade", nil)
	if res == nil || len(res.Content()) == 0 {
		t.Fatalf("expected non-empty circular monogram PNG resource")
	}
	mw.channelIconsCk.SetChecked(true)
	if !sess.ConfigSnapshot().Accessibility.ShowChannelIcons {
		t.Fatalf("expected ShowChannelIcons=true after checking Channel Icons toggle")
	}

	// Verify Speak Emotes toggle in main window
	if sess.ConfigSnapshot().Audio.SpeakEmotes {
		t.Fatalf("expected SpeakEmotes=false by default")
	}
	mw.speakEmotesCk.SetChecked(true)
	if !sess.ConfigSnapshot().Audio.SpeakEmotes {
		t.Fatalf("expected SpeakEmotes=true after checking Speak Emotes toggle")
	}
	mw.speakEmotesCk.SetChecked(false)
	if sess.ConfigSnapshot().Audio.SpeakEmotes {
		t.Fatalf("expected SpeakEmotes=false after unchecking Speak Emotes toggle")
	}

	// Verify 1-Click Mini Player toggle
	if mw.IsMiniMode() {
		t.Fatalf("expected initial IsMiniMode() == false")
	}
	mw.ToggleMiniMode()
	if !mw.IsMiniMode() {
		t.Fatalf("expected IsMiniMode() == true after first toggle")
	}
	mw.ToggleMiniMode()
	if mw.IsMiniMode() {
		t.Fatalf("expected IsMiniMode() == false after second toggle")
	}

	// Verify HearsayTheme font scaling & high contrast
	th := NewHearsayTheme(1.5, true)
	baseSize := NewHearsayTheme(1.0, false).Size(theme.SizeNameText)
	scaledSize := th.Size(theme.SizeNameText)
	if scaledSize <= baseSize {
		t.Fatalf("expected scaled font size (%v) > base size (%v)", scaledSize, baseSize)
	}
}

func TestMainWindowConcurrentUpdatesAndSlotControls(t *testing.T) {
	a := fynetest.NewTempApp(t)
	store := config.NewMemoryStore()
	cfg, _ := store.Load()

	sess := listener.NewSession(cfg, store)
	defer func() { _ = sess.Close() }()

	sess.SetConnectorFactory(func(_ string, _ func(ev *chat.Event), onStatus func(status string, detail string)) (listener.SlotConnector, error) {
		return &noopConnector{onStatus: onStatus}, nil
	})

	mockEnum := func() ([]audio.DeviceInfo, error) {
		return []audio.DeviceInfo{
			{ID: "dev-1", Name: "Headphones (Stereo)", IsDefault: true},
		}, nil
	}

	mw := NewMainWindow(a, sess, mockEnum)

	if err := sess.AddSlot(config.StreamSlotConfig{
		Platform:    "twitch",
		Channel:     "streamer_one",
		DisplayName: "Streamer One",
		Volume:      1.0,
		Pan:         -0.5,
		CrowdFilter: config.FilterSubsVIPsMods,
	}); err != nil {
		t.Fatalf("AddSlot failed: %v", err)
	}

	// Simulate concurrent inbound chat events and UI refreshes under -race
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 10 {
			sess.HandleInboundEvent("twitch:streamer_one", &chat.Event{
				Kind:     chat.EventKindMessage,
				UserName: "SubUser",
				Roles:    []string{"subscriber"},
				Text:     "Testing concurrent live log updates!",
			})
		}
	}()
	<-done

	mw.refreshMixerAndLibrary()
	mw.refreshLog()

	if len(sess.LogSnapshot()) != 10 {
		t.Fatalf("expected 10 log entries, got %d", len(sess.LogSnapshot()))
	}

	// Verify TooltipButton hover and unhover behavior under uiMu without pushing modal PopUp overlays
	if mw.pauseBtn.Tooltip == "" || mw.panicBtn.Tooltip == "" {
		t.Fatalf("expected tooltips on transport and header buttons")
	}
	mw.showTooltip(mw.pauseBtn, mw.pauseBtn.Tooltip)
	if overlays := len(mw.Window.Canvas().Overlays().List()); overlays != 0 {
		t.Fatalf("expected 0 modal canvas overlays when showing tooltip, got %d", overlays)
	}
	mw.hideTooltip(mw.pauseBtn)
}

func TestParseHotkey(t *testing.T) {
	spec, err := ParseHotkey("Ctrl+Shift+P")
	if err != nil {
		t.Fatalf("ParseHotkey failed: %v", err)
	}
	if !spec.Ctrl || !spec.Shift || spec.Key != "P" {
		t.Fatalf("unexpected HotkeySpec: %+v", spec)
	}
	if _, err := ParseHotkey("A"); err == nil {
		t.Fatalf("expected error for unmodified single letter hotkey")
	}
}
