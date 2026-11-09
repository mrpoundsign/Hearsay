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
	mw.QuickTuneEntry.SetText("https://www.twitch.tv/piratesoftware")
	fynetest.Tap(mw.TuneInBtn)

	slots := sess.SlotsSnapshot()
	if len(slots) != 1 || slots[0].Config.Channel != "piratesoftware" {
		t.Fatalf("expected 1 tuned slot for piratesoftware, got %+v", slots)
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
