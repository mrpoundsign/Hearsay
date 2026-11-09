package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mrpoundsign/TownCrier/pkg/tts"
)

const (
	AppID = "com.github.mrpoundsign.hearsay"

	FilterAll                  = "all"
	FilterSubsVIPsMods         = "subs_vips_mods"
	FilterModsStreamerMentions = "mods_streamer_mentions"
)

// Config represents the persistent root configuration for Hearsay.
type Config struct {
	Audio          AudioConfig         `json:"audio"`
	Identity       IdentityConfig      `json:"identity"`
	Crowd          CrowdConfig         `json:"crowd"`
	Accessibility  AccessibilityConfig `json:"accessibility"`
	Translation    TranslationConfig   `json:"translation"`
	SavedStreamers []SavedStreamer     `json:"saved_streamers"`
	Lineups        []Lineup            `json:"lineups"`
	ActiveSlots    []StreamSlotConfig  `json:"active_slots"`
}

// AudioConfig configures audio output, neural TTS models, and voice behavior.
type AudioConfig struct {
	OutputDeviceID       string   `json:"output_device_id"`
	MasterVolume         float64  `json:"master_volume"`
	DefaultSpeed         float64  `json:"default_speed"`
	DefaultVoice         string   `json:"default_voice"`
	EnabledModels        []string `json:"enabled_models"`
	UseGPU               bool     `json:"use_gpu"`
	NumThreads           int      `json:"num_threads"`
	HashChatterVoices    bool     `json:"hash_chatter_voices"`
	SmartGenderVoices    bool     `json:"smart_gender_voices"`
	UnknownGenderPolicy  string   `json:"unknown_gender_policy"`
	SpeakChannelOnSwitch bool     `json:"speak_channel_on_switch"`
	SpeakNames           bool     `json:"speak_names"`
	NaturalizeNames      bool     `json:"naturalize_names"`
	CoalesceSeconds      int      `json:"coalesce_seconds"`
	SpeakEmotes          bool     `json:"speak_emotes"`
	DeduplicateEmotes    bool     `json:"deduplicate_emotes"`
	MaxDuplicateWords    int      `json:"max_duplicate_words"`
	EarconsEnabled       bool     `json:"earcons_enabled"`
}

// EffectiveEnabledModels returns a non-empty list of enabled TTS model families.
func (a AudioConfig) EffectiveEnabledModels() []string {
	if len(a.EnabledModels) == 0 {
		return []string{tts.ModelSupertonic}
	}
	return a.EnabledModels
}

// IdentityConfig stores zero-login plain-text usernames, priority friends, muted chatters, and voice overrides.
type IdentityConfig struct {
	MyUsernames      []string          `json:"my_usernames"`
	PriorityChatters []string          `json:"priority_chatters"`
	IgnoredChatters  []string          `json:"ignored_chatters"`
	ChatterVoices    map[string]string `json:"chatter_voices"`
}

// CrowdConfig configures global crowd control and high-velocity chat filtering.
type CrowdConfig struct {
	DefaultFilterMode  string `json:"default_filter_mode"`
	ChatterCooldownSec int    `json:"chatter_cooldown_sec"`
	GlobalPacingMs     int    `json:"global_pacing_ms"`
	SkipShortSpam      bool   `json:"skip_short_spam"`
	SpeakNoTextEvents  bool   `json:"speak_no_text_events"`
}

// AccessibilityConfig configures UI scaling, high-contrast mode, mini player, and global hotkeys.
type AccessibilityConfig struct {
	UIScale          string       `json:"ui_scale"`
	FontScale        float32      `json:"font_scale"`
	HighContrast     bool         `json:"high_contrast"`
	ShowChannelIcons bool         `json:"show_channel_icons"`
	AlwaysOnTopMini  bool         `json:"always_on_top_mini"`
	Hotkeys          HotkeyConfig `json:"hotkeys"`
}

// HotkeyConfig defines the global keyboard shortcuts for Hearsay.
type HotkeyConfig struct {
	PauseResume string `json:"pause_resume"`
	SkipCurrent string `json:"skip_current"`
	ReplayLast  string `json:"replay_last"`
	PanicMute   string `json:"panic_mute"`
}

// TranslationConfig configures optional automatic chat translation before speech synthesis.
type TranslationConfig struct {
	Enabled       bool    `json:"enabled"`
	TargetLang    string  `json:"target_lang"`
	MinConfidence float64 `json:"min_confidence"`
	Provider      string  `json:"provider"`
	ContactEmail  string  `json:"contact_email"`
	EndpointURL   string  `json:"endpoint_url"`
	Model         string  `json:"model"`
	APIKey        string  `json:"api_key,omitempty"`
}

// StreamSlotConfig configures a single live stream listener slot.
type StreamSlotConfig struct {
	Platform          string   `json:"platform"`
	Channel           string   `json:"channel"`
	DisplayName       string   `json:"display_name,omitempty"`
	Voice             string   `json:"voice,omitempty"`
	VoicePool         []string `json:"voice_pool,omitempty"`
	Volume            float64  `json:"volume"`
	Pan               float64  `json:"pan"`
	Muted             bool     `json:"muted"`
	CrowdFilter       string   `json:"crowd_filter,omitempty"`
	SpeakNoTextEvents *bool    `json:"speak_no_text_events,omitempty"`
}

// SlotID returns a canonical lowercase identifier for the stream slot ("platform:channel").
func (s StreamSlotConfig) SlotID() string {
	return strings.ToLower(strings.TrimSpace(s.Platform)) + ":" + strings.ToLower(strings.TrimSpace(s.Channel))
}

// EffectiveLabel returns DisplayName if set, or Channel otherwise.
func (s StreamSlotConfig) EffectiveLabel() string {
	if strings.TrimSpace(s.DisplayName) != "" {
		return strings.TrimSpace(s.DisplayName)
	}
	return strings.TrimSpace(s.Channel)
}

// SavedStreamer represents a bookmarked creator (single channel or multi-platform channels).
type SavedStreamer struct {
	ID          string             `json:"id"`
	DisplayName string             `json:"display_name"`
	Channels    []StreamSlotConfig `json:"channels"`
}

// Lineup represents a saved multi-stream preset (e.g., co-streams or tournament views with stereo panning).
type Lineup struct {
	ID    string             `json:"id"`
	Name  string             `json:"name"`
	Slots []StreamSlotConfig `json:"slots"`
}

// DefaultConfig returns a new Config populated with sensible accessibility defaults.
func DefaultConfig() *Config {
	return &Config{
		Audio: AudioConfig{
			OutputDeviceID:       "",
			MasterVolume:         1.0,
			DefaultSpeed:         1.0,
			DefaultVoice:         "supertonic:f1",
			EnabledModels:        []string{tts.ModelSupertonic},
			UseGPU:               true,
			NumThreads:           4,
			HashChatterVoices:    true,
			SmartGenderVoices:    true,
			UnknownGenderPolicy:  "balanced",
			SpeakChannelOnSwitch: true,
			SpeakNames:           true,
			NaturalizeNames:      true,
			CoalesceSeconds:      30,
			SpeakEmotes:          false,
			DeduplicateEmotes:    true,
			MaxDuplicateWords:    2,
			EarconsEnabled:       true,
		},
		Identity: IdentityConfig{
			MyUsernames:      []string{},
			PriorityChatters: []string{},
			IgnoredChatters:  []string{},
			ChatterVoices:    make(map[string]string),
		},
		Crowd: CrowdConfig{
			DefaultFilterMode:  FilterAll,
			ChatterCooldownSec: 0,
			GlobalPacingMs:     0,
			SkipShortSpam:      true,
			SpeakNoTextEvents:  false,
		},
		Accessibility: AccessibilityConfig{
			UIScale:         "1.0",
			FontScale:       1.0,
			HighContrast:    false,
			AlwaysOnTopMini: true,
			Hotkeys: HotkeyConfig{
				PauseResume: "Ctrl+Shift+P",
				SkipCurrent: "Ctrl+Shift+S",
				ReplayLast:  "Ctrl+Shift+R",
				PanicMute:   "Ctrl+Shift+M",
			},
		},
		Translation: TranslationConfig{
			Enabled:       false,
			TargetLang:    "en",
			MinConfidence: 0.65,
			Provider:      "mymemory",
		},
		SavedStreamers: []SavedStreamer{},
		Lineups:        []Lineup{},
		ActiveSlots:    []StreamSlotConfig{},
	}
}

// Normalize sanitizes and clamps fields on a loaded Config.
func (c *Config) Normalize() {
	if c.Audio.MasterVolume <= 0 || c.Audio.MasterVolume > 2.0 {
		c.Audio.MasterVolume = 1.0
	}
	if c.Audio.DefaultSpeed < 0.5 || c.Audio.DefaultSpeed > 2.5 {
		c.Audio.DefaultSpeed = 1.0
	}
	if strings.TrimSpace(c.Audio.DefaultVoice) == "" {
		c.Audio.DefaultVoice = "supertonic:f1"
	}
	if len(c.Audio.EnabledModels) == 0 {
		c.Audio.EnabledModels = []string{tts.ModelSupertonic}
	}
	if c.Audio.NumThreads <= 0 {
		c.Audio.NumThreads = 4
	}
	if strings.TrimSpace(c.Audio.UnknownGenderPolicy) == "" {
		c.Audio.UnknownGenderPolicy = "balanced"
		c.Audio.SmartGenderVoices = true
	}
	if c.Audio.CoalesceSeconds < 0 {
		c.Audio.CoalesceSeconds = 0
	}
	if c.Audio.MaxDuplicateWords < 0 {
		c.Audio.MaxDuplicateWords = 0
	}
	if c.Identity.ChatterVoices == nil {
		c.Identity.ChatterVoices = make(map[string]string)
	}
	switch c.Crowd.DefaultFilterMode {
	case FilterAll, FilterSubsVIPsMods, FilterModsStreamerMentions:
	default:
		c.Crowd.DefaultFilterMode = FilterAll
	}
	if c.Crowd.ChatterCooldownSec < 0 {
		c.Crowd.ChatterCooldownSec = 0
	}
	if c.Crowd.GlobalPacingMs < 0 {
		c.Crowd.GlobalPacingMs = 0
	}
	if c.Accessibility.FontScale < 0.8 || c.Accessibility.FontScale > 2.5 {
		c.Accessibility.FontScale = 1.0
	}
	switch strings.ToLower(strings.TrimSpace(c.Accessibility.UIScale)) {
	case "0.85", "85%", "0.9", "90%", "1.0", "1", "100%", "1.15", "115%", "1.25", "125%", "1.5", "1.50", "150%", "auto":
	default:
		c.Accessibility.UIScale = "1.0"
	}
	if strings.TrimSpace(c.Accessibility.Hotkeys.PauseResume) == "" {
		c.Accessibility.Hotkeys.PauseResume = "Ctrl+Shift+P"
	}
	if strings.TrimSpace(c.Accessibility.Hotkeys.SkipCurrent) == "" {
		c.Accessibility.Hotkeys.SkipCurrent = "Ctrl+Shift+S"
	}
	if strings.TrimSpace(c.Accessibility.Hotkeys.ReplayLast) == "" {
		c.Accessibility.Hotkeys.ReplayLast = "Ctrl+Shift+R"
	}
	if strings.TrimSpace(c.Accessibility.Hotkeys.PanicMute) == "" {
		c.Accessibility.Hotkeys.PanicMute = "Ctrl+Shift+M"
	}
	if strings.TrimSpace(c.Translation.TargetLang) == "" {
		c.Translation.TargetLang = "en"
	}
	if c.Translation.MinConfidence <= 0 || c.Translation.MinConfidence > 1.0 {
		c.Translation.MinConfidence = 0.65
	}
	if strings.TrimSpace(c.Translation.Provider) == "" {
		c.Translation.Provider = "mymemory"
	}
	for i := range c.ActiveSlots {
		normalizeSlot(&c.ActiveSlots[i])
	}
	for i := range c.SavedStreamers {
		for j := range c.SavedStreamers[i].Channels {
			normalizeSlot(&c.SavedStreamers[i].Channels[j])
		}
	}
	for i := range c.Lineups {
		for j := range c.Lineups[i].Slots {
			normalizeSlot(&c.Lineups[i].Slots[j])
		}
	}
}

func normalizeSlot(s *StreamSlotConfig) {
	s.Platform = strings.ToLower(strings.TrimSpace(s.Platform))
	s.Channel = strings.TrimSpace(s.Channel)
	if s.Volume <= 0 || s.Volume > 2.0 {
		s.Volume = 1.0
	}
	if s.Pan < -1.0 {
		s.Pan = -1.0
	} else if s.Pan > 1.0 {
		s.Pan = 1.0
	}
}

// Clone returns a deep copy of Config.
func (c *Config) Clone() *Config {
	if c == nil {
		return DefaultConfig()
	}
	data, err := json.Marshal(c)
	if err != nil {
		return DefaultConfig()
	}
	cp := DefaultConfig()
	if err := json.Unmarshal(data, cp); err != nil {
		return DefaultConfig()
	}
	cp.Normalize()
	return cp
}

// AddOrUpdateSavedStreamer saves or updates a streamer in SavedStreamers.
func (c *Config) AddOrUpdateSavedStreamer(st SavedStreamer) {
	if strings.TrimSpace(st.ID) == "" {
		st.ID = strings.ToLower(strings.TrimSpace(st.DisplayName))
	}
	for i := range st.Channels {
		normalizeSlot(&st.Channels[i])
	}
	for i, existing := range c.SavedStreamers {
		if strings.EqualFold(existing.ID, st.ID) || strings.EqualFold(existing.DisplayName, st.DisplayName) {
			c.SavedStreamers[i] = st
			return
		}
	}
	c.SavedStreamers = append(c.SavedStreamers, st)
}

// RemoveSavedStreamer deletes a saved streamer by ID.
func (c *Config) RemoveSavedStreamer(id string) {
	c.SavedStreamers = slices.DeleteFunc(c.SavedStreamers, func(s SavedStreamer) bool {
		return strings.EqualFold(s.ID, id)
	})
}

// AddOrUpdateLineup saves or updates a multi-stream lineup preset.
func (c *Config) AddOrUpdateLineup(l Lineup) {
	if strings.TrimSpace(l.ID) == "" {
		l.ID = strings.ToLower(strings.TrimSpace(l.Name))
	}
	for i := range l.Slots {
		normalizeSlot(&l.Slots[i])
	}
	for i, existing := range c.Lineups {
		if strings.EqualFold(existing.ID, l.ID) || strings.EqualFold(existing.Name, l.Name) {
			c.Lineups[i] = l
			return
		}
	}
	c.Lineups = append(c.Lineups, l)
}

// RemoveLineup deletes a lineup preset by ID.
func (c *Config) RemoveLineup(id string) {
	c.Lineups = slices.DeleteFunc(c.Lineups, func(l Lineup) bool {
		return strings.EqualFold(l.ID, id)
	})
}

// Save writes the configuration atomically to the given path.
func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("cannot save nil config")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename temp config: %w", err)
	}
	return nil
}

// Load reads and normalizes the configuration from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- configuration path is explicitly controlled by Locations
	if err != nil {
		return nil, err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config JSON: %w", err)
	}
	cfg.Normalize()
	return cfg, nil
}
