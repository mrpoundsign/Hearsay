package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProductionLocationsForbiddenInTests(t *testing.T) {
	_, err := ProductionLocations()
	if !errors.Is(err, ErrProductionConfigInTest) {
		t.Fatalf("expected ErrProductionConfigInTest, got %v", err)
	}
}

func TestFileStoreRoundTripAndLibraryManagement(t *testing.T) {
	loc := TestLocations(t.TempDir())
	store, err := NewFileStore(loc)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Identity.MyUsernames = []string{"AccessibilityViewer"}
	cfg.AddOrUpdateSavedStreamer(SavedStreamer{
		ID:          "mocomade",
		DisplayName: "MoCoMade",
		Channels: []StreamSlotConfig{
			{Platform: "twitch", Channel: "mocomade", Volume: 1.0, Pan: -0.5},
			{Platform: "youtube", Channel: "@MoCoMade", Volume: 1.0, Pan: 0.5},
		},
	})
	cfg.AddOrUpdateLineup(Lineup{
		ID:   "co-stream",
		Name: "Co-Stream Duo",
		Slots: []StreamSlotConfig{
			{Platform: "twitch", Channel: "streamer_a", Volume: 1.0, Pan: -0.75},
			{Platform: "twitch", Channel: "streamer_b", Volume: 1.0, Pan: 0.75},
		},
	})

	if err := store.Save(cfg); err != nil {
		t.Fatalf("store.Save failed: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load failed: %v", err)
	}

	if len(loaded.Identity.MyUsernames) != 1 || loaded.Identity.MyUsernames[0] != "AccessibilityViewer" {
		t.Fatalf("unexpected MyUsernames: %+v", loaded.Identity.MyUsernames)
	}
	if len(loaded.SavedStreamers) != 1 || len(loaded.SavedStreamers[0].Channels) != 2 {
		t.Fatalf("expected 1 saved streamer with 2 channels, got %+v", loaded.SavedStreamers)
	}
	if len(loaded.Lineups) != 1 || len(loaded.Lineups[0].Slots) != 2 {
		t.Fatalf("expected 1 lineup with 2 slots, got %+v", loaded.Lineups)
	}
	if loaded.Lineups[0].Slots[0].Pan != -0.75 || loaded.Lineups[0].Slots[1].Pan != 0.75 {
		t.Fatalf("unexpected slot pans: %+v", loaded.Lineups[0].Slots)
	}

	loaded.RemoveSavedStreamer("mocomade")
	loaded.RemoveLineup("co-stream")
	if len(loaded.SavedStreamers) != 0 || len(loaded.Lineups) != 0 {
		t.Fatalf("expected saved streamers and lineups to be removed")
	}
}

func TestMemoryStoreIsolation(t *testing.T) {
	store := NewMemoryStore()
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	cfg.Accessibility.FontScale = 1.5
	cfg.Accessibility.HighContrast = true
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	reloaded, err := store.Load()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if reloaded.Accessibility.FontScale != 1.5 || !reloaded.Accessibility.HighContrast {
		t.Fatalf("unexpected accessibility settings: %+v", reloaded.Accessibility)
	}

	// Verify ChatterSpeeds normalization omits 1.0x default and clamps out-of-range multipliers
	cfg.Identity.ChatterSpeeds = map[string]float64{
		"default_user": 1.0,
		"fast_user":    1.25,
		"too_fast":     4.0,
		"too_slow":     0.1,
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save with ChatterSpeeds failed: %v", err)
	}
	reloaded2, err := store.Load()
	if err != nil {
		t.Fatalf("Reload with ChatterSpeeds failed: %v", err)
	}
	if _, exists := reloaded2.Identity.ChatterSpeeds["default_user"]; exists {
		t.Errorf("expected 1.0x default speed multiplier to be omitted on normalize")
	}
	if got := reloaded2.Identity.ChatterSpeeds["fast_user"]; got != 1.25 {
		t.Errorf("expected fast_user multiplier 1.25, got %v", got)
	}
	if got := reloaded2.Identity.ChatterSpeeds["too_fast"]; got != 2.5 {
		t.Errorf("expected too_fast clamped to 2.5, got %v", got)
	}
	if got := reloaded2.Identity.ChatterSpeeds["too_slow"]; got != 0.5 {
		t.Errorf("expected too_slow clamped to 0.5, got %v", got)
	}
}

func TestUpdateConfigDefaultsAndRoundTrip(t *testing.T) {
	loc := TestLocations(t.TempDir())
	store, err := NewFileStore(loc)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	// Legacy JSON without an "update" key defaults CheckOnStartup to true.
	if err := os.MkdirAll(filepath.Dir(loc.ConfigFile), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(loc.ConfigFile, []byte(`{"audio":{"master_volume":1.0}}`), 0o600); err != nil {
		t.Fatalf("WriteFile legacy config failed: %v", err)
	}
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("initial Load failed: %v", err)
	}
	if !cfg.Update.CheckOnStartup {
		t.Errorf("expected Update.CheckOnStartup to default to true")
	}
	if cfg.Update.SkippedVersion != "" {
		t.Errorf("expected empty SkippedVersion by default, got %q", cfg.Update.SkippedVersion)
	}

	cfg.Update.CheckOnStartup = false
	cfg.Update.SkippedVersion = "0.0.5"
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	reloaded, err := store.Load()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if reloaded.Update.CheckOnStartup {
		t.Errorf("expected Update.CheckOnStartup false after round-trip")
	}
	if reloaded.Update.SkippedVersion != "0.0.5" {
		t.Errorf("expected SkippedVersion '0.0.5', got %q", reloaded.Update.SkippedVersion)
	}
}
