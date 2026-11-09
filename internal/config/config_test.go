package config

import (
	"errors"
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
}
