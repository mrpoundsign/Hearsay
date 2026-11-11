package listener

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/TownCrier/pkg/audio"
	"github.com/mrpoundsign/TownCrier/pkg/bots"
	"github.com/mrpoundsign/TownCrier/pkg/chat"
	tcConfig "github.com/mrpoundsign/TownCrier/pkg/config"
	"github.com/mrpoundsign/TownCrier/pkg/platform/kick"
	"github.com/mrpoundsign/TownCrier/pkg/platform/tiktok"
	"github.com/mrpoundsign/TownCrier/pkg/platform/twitch"
	"github.com/mrpoundsign/TownCrier/pkg/platform/youtube"
	"github.com/mrpoundsign/TownCrier/pkg/translate"
	"github.com/mrpoundsign/TownCrier/pkg/tts"
)

const (
	MaxLogEntries   = 500
	MaxSpeechQueue  = 64
	StaleMessageTTL = 30 * time.Second
)

// LogStatus represents the lifecycle state of a message in the Unified Live Log.
type LogStatus string

const (
	StatusQueued   LogStatus = "queued"
	StatusSpeaking LogStatus = "speaking"
	StatusSpoken   LogStatus = "spoken"
	StatusSkipped  LogStatus = "skipped"
)

// LogEntry represents an entry in the in-memory Unified Live Log ring buffer.
type LogEntry struct {
	ID         uint64
	SlotID     string
	Platform   string
	Channel    string
	UserName   string
	UserID     string
	UserColor  string
	Text       string
	SpokenText string
	VoiceID    string
	Pan        float64
	Volume     float64
	Status     LogStatus
	SkipReason string
	Coalesced  bool
	IsMention  bool
	IsPriority bool
	Timestamp  time.Time
}

// SlotState represents the live runtime state of an active stream slot.
type SlotState struct {
	Config       config.StreamSlotConfig
	Status       string // "disconnected", "connecting", "waiting", "connected"
	StatusDetail string
}

// SlotConnector abstracts a platform stream connector instance.
type SlotConnector interface {
	Start(target string) error
	Stop() error
}

// ConnectorFactory creates a SlotConnector for a given platform and channel.
type ConnectorFactory func(
	platform string,
	onEvent func(ev *chat.Event),
	onStatus func(status string, detail string),
) (SlotConnector, error)

type activeSlot struct {
	cfg       config.StreamSlotConfig
	status    string
	detail    string
	connector SlotConnector
}

type speechItem struct {
	entryID    uint64
	slotID     string
	spokenText string
	voiceID    string
	speed      float64
	volume     float64
	pan        float64
	isMention  bool
	isPriority bool
	enqueuedAt time.Time
}

// Session coordinates multi-stream connectors, Crowd Control filtering, TTS synthesis,
// stereo panned audio playback, and the Unified Live Log.
type Session struct {
	mu               sync.RWMutex
	cfg              *config.Config
	store            config.Store
	player           audio.Player
	engine           tts.Engine
	sanitizer        *chat.Sanitizer
	botMatcher       *bots.Matcher
	botConfig        *tcConfig.Config
	translator       *translate.Service
	pronouns         *chat.PronounCache
	crowd            *CrowdController
	connectorFactory ConnectorFactory

	slots            map[string]*activeSlot
	slotOrder        []string
	lastSpokenSlotID string
	lastSpeakerKey   string
	lastSpokenAt     time.Time

	logEntries    []LogEntry
	nextEntryID   atomic.Uint64
	lastSpoken    *LogEntry
	nowSpeaking   *LogEntry
	paused        atomic.Bool
	panicked      atomic.Bool
	closed        atomic.Bool
	speechCh      chan speechItem
	wakeCh        chan struct{}
	workerStopCh  chan struct{}
	workerStopped chan struct{}

	onLogUpdated   func()
	onSlotsUpdated func()
	onNowSpeaking  func(entry *LogEntry)
}

// NewSession creates a new multi-stream listener Session.
func NewSession(cfg *config.Config, store config.Store) *Session {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	cfg.Normalize()

	matcher, _ := bots.NewMatcher()
	botCfg := tcConfig.DefaultConfig()
	botCfg.Bots.IgnoreKnownBots = true

	transSvc := translate.NewService()
	pronounCache := chat.NewPronounCache()
	if !testing.Testing() {
		if sharedCache := tts.DefaultSharedCacheDir(); sharedCache != "" {
			transSvc.SetDiskCachePath(filepath.Join(sharedCache, "translations.json"))
			pronounCache.SetDiskCachePath(filepath.Join(sharedCache, "pronouns.json"))
		}
	}

	s := &Session{
		cfg:           cfg,
		store:         store,
		sanitizer:     chat.NewSanitizer(chat.DefaultMaxRunes),
		botMatcher:    matcher,
		botConfig:     botCfg,
		translator:    transSvc,
		pronouns:      pronounCache,
		crowd:         NewCrowdController(),
		slots:         make(map[string]*activeSlot),
		speechCh:      make(chan speechItem, MaxSpeechQueue),
		wakeCh:        make(chan struct{}, 1),
		workerStopCh:  make(chan struct{}),
		workerStopped: make(chan struct{}),
	}
	s.connectorFactory = defaultConnectorFactory

	go s.speechWorker()
	return s
}

func defaultConnectorFactory(
	platform string,
	onEvent func(ev *chat.Event),
	onStatus func(status string, detail string),
) (SlotConnector, error) {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "twitch":
		conn := twitch.NewConnector(
			onEvent,
			nil,
			func(st twitch.ConnectionStatus, detail string) {
				onStatus(string(st), detail)
			},
		)
		return conn, nil
	case "youtube":
		conn := youtube.NewConnector(
			onEvent,
			nil,
			func(st youtube.ConnectionStatus, detail string) {
				onStatus(string(st), detail)
			},
		)
		return conn, nil
	case "kick":
		conn := kick.NewConnector(
			onEvent,
			nil,
			func(st kick.ConnectionStatus, detail string) {
				onStatus(string(st), detail)
			},
		)
		return conn, nil
	case "tiktok":
		conn := tiktok.NewConnector(
			onEvent,
			func(st tiktok.ConnectionStatus, detail string) {
				onStatus(string(st), detail)
			},
		)
		return conn, nil
	default:
		return nil, errors.New("unsupported platform: " + platform)
	}
}

// SetConnectorFactory overrides the platform connector factory (used in unit tests).
func (s *Session) SetConnectorFactory(fn ConnectorFactory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fn != nil {
		s.connectorFactory = fn
	}
}

// SetPlayer binds the audio playback device to the session.
func (s *Session) SetPlayer(p audio.Player) {
	s.mu.Lock()
	s.player = p
	vol := s.cfg.Audio.MasterVolume
	s.mu.Unlock()
	if p != nil && vol > 0 {
		p.SetVolume(float32(vol))
	}
}

// Player returns the bound audio player.
func (s *Session) Player() audio.Player {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.player
}

// SetEngine binds the neural TTS engine to the session.
func (s *Session) SetEngine(e tts.Engine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.engine = e
}

// Engine returns the bound neural TTS engine.
func (s *Session) Engine() tts.Engine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.engine
}

// SetCallbacks registers UI refresh callbacks for log updates, slot state changes, and now-speaking banner changes.
func (s *Session) SetCallbacks(onLog func(), onSlots func(), onSpeaking func(entry *LogEntry)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLogUpdated = onLog
	s.onSlotsUpdated = onSlots
	s.onNowSpeaking = onSpeaking
}

// ConfigSnapshot returns a deep copy of the current configuration.
func (s *Session) ConfigSnapshot() *config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Clone()
}

// UpdateConfig applies a mutation function to the configuration and persists it to the store.
func (s *Session) UpdateConfig(mutator func(cfg *config.Config)) error {
	s.mu.Lock()
	prevDevID := s.cfg.Audio.OutputDeviceID
	mutator(s.cfg)
	s.cfg.Normalize()
	cp := s.cfg.Clone()
	p := s.player
	store := s.store
	s.mu.Unlock()

	if p != nil {
		if cp.Audio.OutputDeviceID != prevDevID {
			if err := p.SwitchDevice(cp.Audio.OutputDeviceID); err != nil {
				log.Printf("[Session] Warning: failed to switch audio output device: %v", err)
			}
		}
		p.SetVolume(float32(cp.Audio.MasterVolume))
	}
	if store != nil {
		return store.Save(cp)
	}
	return nil
}

// Start restores and connects all ActiveSlots persisted in the configuration.
func (s *Session) Start() error {
	s.mu.RLock()
	initialSlots := slices.Clone(s.cfg.ActiveSlots)
	s.mu.RUnlock()

	for _, sc := range initialSlots {
		if err := s.AddSlot(sc); err != nil {
			log.Printf("[Session] Warning: failed to start persisted slot %s: %v", sc.SlotID(), err)
		}
	}
	return nil
}

// TuneIn parses a raw stream URL, "platform:channel" shorthand, or handle and connects a new stream slot.
func (s *Session) TuneIn(rawInput string, fallbackPlatform string) (config.StreamSlotConfig, error) {
	plat, ch, err := ParseTargetInput(rawInput, fallbackPlatform)
	if err != nil {
		return config.StreamSlotConfig{}, err
	}
	slotCfg := config.StreamSlotConfig{
		Platform:    plat,
		Channel:     ch,
		DisplayName: ch,
		Volume:      1.0,
		Pan:         s.suggestNextStereoPan(),
	}
	if err := s.AddSlot(slotCfg); err != nil {
		return config.StreamSlotConfig{}, err
	}
	return slotCfg, nil
}

// suggestNextStereoPan automatically suggests balanced stereo panning when adding streams:
// 1st stream -> Center (0.0), 2nd stream -> Left (-0.6) or Right (+0.6), etc.
func (s *Session) suggestNextStereoPan() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := len(s.slotOrder)
	switch count {
	case 0:
		return 0.0
	case 1:
		return 0.65
	case 2:
		return -0.65
	default:
		return 0.0
	}
}

// AddSlot connects and adds a stream slot (or updates it if already active).
func (s *Session) AddSlot(slotCfg config.StreamSlotConfig) error {
	slotCfg.Platform = strings.ToLower(strings.TrimSpace(slotCfg.Platform))
	slotCfg.Channel = strings.TrimSpace(slotCfg.Channel)
	if slotCfg.Platform == "" || slotCfg.Channel == "" {
		return errors.New("platform and channel are required")
	}
	if slotCfg.Volume <= 0 || slotCfg.Volume > 2.0 {
		slotCfg.Volume = 1.0
	}
	slotID := slotCfg.SlotID()

	s.mu.Lock()
	if existing, ok := s.slots[slotID]; ok {
		existing.cfg = slotCfg
		s.syncActiveSlotsLocked()
		s.mu.Unlock()
		s.notifySlotsUpdated()
		return nil
	}
	factory := s.connectorFactory
	s.mu.Unlock()

	conn, err := factory(
		slotCfg.Platform,
		func(ev *chat.Event) {
			s.HandleInboundEvent(slotID, ev)
		},
		func(status string, detail string) {
			s.handleSlotStatusChange(slotID, status, detail)
		},
	)
	if err != nil {
		return err
	}

	as := &activeSlot{
		cfg:       slotCfg,
		status:    "connecting",
		detail:    "Connecting...",
		connector: conn,
	}

	s.mu.Lock()
	s.slots[slotID] = as
	s.slotOrder = append(s.slotOrder, slotID)
	s.syncActiveSlotsLocked()
	cp := s.cfg.Clone()
	store := s.store
	s.mu.Unlock()

	if store != nil {
		_ = store.Save(cp)
	}
	s.notifySlotsUpdated()

	if err := conn.Start(slotCfg.Channel); err != nil {
		s.handleSlotStatusChange(slotID, "disconnected", err.Error())
		return err
	}
	return nil
}

// RemoveSlot disconnects and removes an active stream slot by its SlotID.
func (s *Session) RemoveSlot(slotID string) error {
	key := strings.ToLower(strings.TrimSpace(slotID))

	s.mu.Lock()
	as, ok := s.slots[key]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.slots, key)
	s.slotOrder = slices.DeleteFunc(s.slotOrder, func(id string) bool {
		return id == key
	})
	s.syncActiveSlotsLocked()
	cp := s.cfg.Clone()
	store := s.store
	s.mu.Unlock()

	if as.connector != nil {
		_ = as.connector.Stop()
	}
	if store != nil {
		_ = store.Save(cp)
	}
	s.notifySlotsUpdated()
	return nil
}

// UpdateSlot modifies an active slot's mixer/filter configuration and persists the change.
func (s *Session) UpdateSlot(slotID string, mutator func(sc *config.StreamSlotConfig)) {
	key := strings.ToLower(strings.TrimSpace(slotID))

	s.mu.Lock()
	as, ok := s.slots[key]
	if !ok {
		s.mu.Unlock()
		return
	}
	mutator(&as.cfg)
	if as.cfg.Pan < -1.0 {
		as.cfg.Pan = -1.0
	} else if as.cfg.Pan > 1.0 {
		as.cfg.Pan = 1.0
	}
	if as.cfg.Volume < 0 {
		as.cfg.Volume = 0
	} else if as.cfg.Volume > 2.0 {
		as.cfg.Volume = 2.0
	}
	s.syncActiveSlotsLocked()
	cp := s.cfg.Clone()
	store := s.store
	s.mu.Unlock()

	if store != nil {
		_ = store.Save(cp)
	}
}

// ActivateStreamer connects all channels associated with a SavedStreamer.
func (s *Session) ActivateStreamer(st config.SavedStreamer) error {
	var firstErr error
	for _, ch := range st.Channels {
		if ch.DisplayName == "" {
			ch.DisplayName = st.DisplayName
		}
		if err := s.AddSlot(ch); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ActivateLineup activates a saved multi-stream Lineup preset.
// If replaceExisting is true, currently active streams not in the lineup are disconnected first.
func (s *Session) ActivateLineup(lineup config.Lineup, replaceExisting bool) error {
	if replaceExisting {
		current := s.SlotsSnapshot()
		for _, st := range current {
			_ = s.RemoveSlot(st.Config.SlotID())
		}
	}
	var firstErr error
	for _, slotCfg := range lineup.Slots {
		if err := s.AddSlot(slotCfg); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SaveCurrentAsLineup saves the currently active stream slots (including their stereo panning and volume) as a named Lineup.
func (s *Session) SaveCurrentAsLineup(name string) (config.Lineup, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return config.Lineup{}, errors.New("lineup name cannot be empty")
	}
	slots := s.SlotsSnapshot()
	if len(slots) == 0 {
		return config.Lineup{}, errors.New("no active streams to save in lineup")
	}
	cfgs := make([]config.StreamSlotConfig, 0, len(slots))
	for _, st := range slots {
		cfgs = append(cfgs, st.Config)
	}
	lineup := config.Lineup{
		ID:    strings.ToLower(trimmed),
		Name:  trimmed,
		Slots: cfgs,
	}
	err := s.UpdateConfig(func(cfg *config.Config) {
		cfg.AddOrUpdateLineup(lineup)
	})
	return lineup, err
}

// SlotsSnapshot returns an ordered snapshot of all active stream slots.
func (s *Session) SlotsSnapshot() []SlotState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SlotState, 0, len(s.slotOrder))
	for _, id := range s.slotOrder {
		if as, ok := s.slots[id]; ok {
			out = append(out, SlotState{
				Config:       as.cfg,
				Status:       as.status,
				StatusDetail: as.detail,
			})
		}
	}
	return out
}

func (s *Session) syncActiveSlotsLocked() {
	active := make([]config.StreamSlotConfig, 0, len(s.slotOrder))
	for _, id := range s.slotOrder {
		if as, ok := s.slots[id]; ok {
			active = append(active, as.cfg)
		}
	}
	s.cfg.ActiveSlots = active
}

func (s *Session) handleSlotStatusChange(slotID string, status string, detail string) {
	s.mu.Lock()
	as, ok := s.slots[slotID]
	if !ok {
		s.mu.Unlock()
		return
	}
	prevStatus := as.status
	as.status = status
	as.detail = detail
	earconsOn := s.cfg.Audio.EarconsEnabled
	pan := as.cfg.Pan
	p := s.player
	s.mu.Unlock()

	if earconsOn && p != nil && prevStatus != status {
		switch status {
		case "connected":
			s.playEarconAsync(audio.EarconStreamOnline, pan)
		case "disconnected":
			if prevStatus == "connected" {
				s.playEarconAsync(audio.EarconStreamOffline, pan)
			}
		}
	}

	s.notifySlotsUpdated()
}

func (s *Session) playEarconAsync(kind audio.EarconKind, pan float64) {
	if s.panicked.Load() {
		return
	}
	s.mu.RLock()
	p := s.player
	s.mu.RUnlock()
	if p == nil {
		return
	}
	const sr = 24000
	samples := audio.SynthesizeEarcon(kind, sr)
	if len(samples) == 0 {
		return
	}
	go func() {
		_ = p.PlayWithOptions(samples, sr, audio.PlayOptions{
			Gain: 0.7,
			Pan:  float32(pan),
		})
	}()
}

// HandleInboundEvent processes an incoming chat or stream event for the specified slot.
func (s *Session) HandleInboundEvent(slotID string, ev *chat.Event) {
	if ev == nil || s.closed.Load() {
		return
	}

	s.mu.RLock()
	as, ok := s.slots[strings.ToLower(strings.TrimSpace(slotID))]
	if !ok {
		s.mu.RUnlock()
		return
	}
	slotCfg := as.cfg
	cfgSnap := s.cfg.Clone()
	activeCount := len(s.slotOrder)
	lastSlot := s.lastSpokenSlotID
	lastSpeaker := s.lastSpeakerKey
	lastAt := s.lastSpokenAt
	eng := s.engine
	s.mu.RUnlock()

	if ev.Platform == "" {
		ev.Platform = slotCfg.Platform
	}
	if ev.Channel == "" {
		ev.Channel = slotCfg.Channel
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}

	// 1. Known Bot Matcher check
	if s.botMatcher != nil && s.botMatcher.ShouldIgnore(ev, s.botConfig) {
		return
	}

	// 2. Clean & sanitize text
	cleanedText := s.sanitizer.SanitizeWithOptions(ev.Text, chat.SanitizeOptions{
		EmoteSpans:        ev.EmoteSpans,
		SpeakEmotes:       cfgSnap.Audio.SpeakEmotes,
		DeduplicateEmotes: cfgSnap.Audio.DeduplicateEmotes,
		MaxDuplicateWords: cfgSnap.Audio.MaxDuplicateWords,
	})

	// 3. Optional Translation
	if cfgSnap.Translation.Enabled && strings.TrimSpace(cleanedText) != "" && s.translator != nil {
		ctx, cancel := context.WithTimeout(context.Background(), translate.DefaultTimeout)
		res, err := s.translator.Translate(ctx, cleanedText, translate.Options{
			Provider:     cfgSnap.Translation.Provider,
			TargetLang:   cfgSnap.Translation.TargetLang,
			ContactEmail: cfgSnap.Translation.ContactEmail,
			EndpointURL:  cfgSnap.Translation.EndpointURL,
			Model:        cfgSnap.Translation.Model,
			APIKey:       cfgSnap.Translation.APIKey,
		})
		cancel()
		if err == nil && strings.TrimSpace(res.TranslatedText) != "" {
			cleanedText = strings.TrimSpace(res.TranslatedText)
		}
	}

	// 4. Crowd Control evaluation
	now := time.Now()
	eval := s.crowd.Evaluate(*ev, cleanedText, slotCfg, cfgSnap, now)

	var availVoices []string
	if eng != nil {
		availVoices = eng.Voices()
	}
	voiceID := SelectChatterVoiceWithPronouns(*ev, slotCfg, cfgSnap, availVoices, s.pronouns)

	entryID := s.nextEntryID.Add(1)
	speaker := chatterName(*ev)
	if speaker == "" {
		speaker = "chat"
	}
	userColor := ResolveChatterColorHex(slotCfg.Platform, speaker, ExtractUserColor(*ev))

	if !eval.Allow {
		s.appendLogEntry(LogEntry{
			ID:         entryID,
			SlotID:     slotCfg.SlotID(),
			Platform:   slotCfg.Platform,
			Channel:    slotCfg.EffectiveLabel(),
			UserName:   speaker,
			UserID:     ev.UserID,
			UserColor:  userColor,
			Text:       ev.Text,
			SpokenText: cleanedText,
			VoiceID:    voiceID,
			Pan:        slotCfg.Pan,
			Volume:     slotCfg.Volume,
			Status:     StatusSkipped,
			SkipReason: eval.Reason,
			IsMention:  eval.IsMention,
			IsPriority: eval.IsPriority,
			Timestamp:  ev.Timestamp,
		})
		return
	}

	if s.panicked.Load() {
		s.appendLogEntry(LogEntry{
			ID:         entryID,
			SlotID:     slotCfg.SlotID(),
			Platform:   slotCfg.Platform,
			Channel:    slotCfg.EffectiveLabel(),
			UserName:   speaker,
			UserID:     ev.UserID,
			UserColor:  userColor,
			Text:       ev.Text,
			SpokenText: cleanedText,
			VoiceID:    voiceID,
			Pan:        slotCfg.Pan,
			Volume:     slotCfg.Volume,
			Status:     StatusSkipped,
			SkipReason: "panic_muted",
			IsMention:  eval.IsMention,
			IsPriority: eval.IsPriority,
			Timestamp:  ev.Timestamp,
		})
		return
	}

	spokenText, nextLastSlot, nextSpeakerKey, coalesced := FormatSpokenTextWithCoalesce(*ev, cleanedText, slotCfg, cfgSnap, activeCount, lastSlot, lastSpeaker, lastAt, now)
	s.mu.Lock()
	s.lastSpokenSlotID = nextLastSlot
	s.lastSpeakerKey = nextSpeakerKey
	s.lastSpokenAt = now
	s.mu.Unlock()

	entry := LogEntry{
		ID:         entryID,
		SlotID:     slotCfg.SlotID(),
		Platform:   slotCfg.Platform,
		Channel:    slotCfg.EffectiveLabel(),
		UserName:   speaker,
		UserID:     ev.UserID,
		UserColor:  userColor,
		Text:       ev.Text,
		SpokenText: spokenText,
		VoiceID:    voiceID,
		Pan:        slotCfg.Pan,
		Volume:     slotCfg.Volume,
		Status:     StatusQueued,
		Coalesced:  coalesced,
		IsMention:  eval.IsMention,
		IsPriority: eval.IsPriority,
		Timestamp:  ev.Timestamp,
	}
	s.appendLogEntry(entry)

	item := speechItem{
		entryID:    entryID,
		slotID:     slotCfg.SlotID(),
		spokenText: spokenText,
		voiceID:    voiceID,
		speed:      cfgSnap.Audio.DefaultSpeed,
		volume:     slotCfg.Volume,
		pan:        slotCfg.Pan,
		isMention:  eval.IsMention,
		isPriority: eval.IsPriority,
		enqueuedAt: time.Now(),
	}

	select {
	case s.speechCh <- item:
	default:
		// Queue full: mark skipped
		s.updateLogStatus(entryID, StatusSkipped, "queue_overflow")
	}
}

func (s *Session) speechWorker() {
	defer close(s.workerStopped)

	for {
		select {
		case <-s.workerStopCh:
			return
		default:
		}

		if s.paused.Load() {
			select {
			case <-s.workerStopCh:
				return
			case <-s.wakeCh:
				continue
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		select {
		case <-s.workerStopCh:
			return
		case item := <-s.speechCh:
			s.processSpeechItem(item)
		}
	}
}

func (s *Session) processSpeechItem(item speechItem) {
	// Wait if paused right before synthesis
	for s.paused.Load() {
		select {
		case <-s.workerStopCh:
			return
		case <-s.wakeCh:
		case <-time.After(100 * time.Millisecond):
		}
	}

	if s.panicked.Load() {
		s.updateLogStatus(item.entryID, StatusSkipped, "panic_muted")
		return
	}

	// Staleness protection: drop non-priority messages older than 30s during bursts
	if !item.isPriority && time.Since(item.enqueuedAt) > StaleMessageTTL {
		s.updateLogStatus(item.entryID, StatusSkipped, "stale_ttl")
		return
	}

	s.mu.RLock()
	eng := s.engine
	p := s.player
	earconsOn := s.cfg.Audio.EarconsEnabled
	baseSpeed := s.cfg.Audio.DefaultSpeed
	s.mu.RUnlock()

	speed := item.speed
	if speed <= 0 {
		speed = baseSpeed
	}
	// Fast catchup mode if speech queue depth > 5
	if len(s.speechCh) > 5 && speed < 1.3 {
		speed = 1.3
	}

	entryCopy := s.updateLogStatus(item.entryID, StatusSpeaking, "")
	if entryCopy != nil {
		s.mu.Lock()
		s.nowSpeaking = entryCopy
		s.lastSpoken = entryCopy
		s.mu.Unlock()
		s.notifyNowSpeaking(entryCopy)
	}

	if item.isMention && earconsOn && p != nil {
		const sr = 24000
		samples := audio.SynthesizeEarcon(audio.EarconMention, sr)
		if len(samples) > 0 {
			_ = p.PlayWithOptions(samples, sr, audio.PlayOptions{
				Gain: float32(item.volume),
				Pan:  float32(item.pan),
			})
		}
	}

	if eng != nil && p != nil && strings.TrimSpace(item.spokenText) != "" {
		samples, sampleRate, err := eng.Synthesize(item.spokenText, item.voiceID, float32(speed))
		if s.panicked.Load() {
			s.updateLogStatus(item.entryID, StatusSkipped, "panic_muted")
			s.mu.Lock()
			s.nowSpeaking = nil
			s.mu.Unlock()
			s.notifyNowSpeaking(nil)
			return
		}
		if err == nil && len(samples) > 0 {
			_ = p.PlayWithOptions(samples, sampleRate, audio.PlayOptions{
				Gain: float32(item.volume),
				Pan:  float32(item.pan),
			})
		}
	}

	s.updateLogStatus(item.entryID, StatusSpoken, "")
	s.mu.Lock()
	s.nowSpeaking = nil
	s.mu.Unlock()
	s.notifyNowSpeaking(nil)
}

func (s *Session) appendLogEntry(entry LogEntry) {
	s.mu.Lock()
	if len(s.logEntries) >= MaxLogEntries {
		s.logEntries = append(s.logEntries[1:], entry)
	} else {
		s.logEntries = append(s.logEntries, entry)
	}
	s.mu.Unlock()
	s.notifyLogUpdated()
}

func (s *Session) updateLogStatus(entryID uint64, status LogStatus, reason string) *LogEntry {
	s.mu.Lock()
	var updated *LogEntry
	for i := len(s.logEntries) - 1; i >= 0; i-- {
		if s.logEntries[i].ID == entryID {
			s.logEntries[i].Status = status
			if reason != "" {
				s.logEntries[i].SkipReason = reason
			}
			cp := s.logEntries[i]
			updated = &cp
			break
		}
	}
	s.mu.Unlock()
	s.notifyLogUpdated()
	return updated
}

// LogSnapshot returns a snapshot of the Unified Live Log entries (newest last).
func (s *Session) LogSnapshot() []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.logEntries)
}

// NowSpeaking returns the currently speaking LogEntry, or nil if idle.
func (s *Session) NowSpeaking() *LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.nowSpeaking == nil {
		return nil
	}
	cp := *s.nowSpeaking
	return &cp
}

// SetPaused pauses or resumes the speech queue.
func (s *Session) SetPaused(paused bool) {
	s.paused.Store(paused)
	if !paused {
		select {
		case s.wakeCh <- struct{}{}:
		default:
		}
	}
}

// TogglePause toggles the paused state of the speech queue and returns the new paused state.
func (s *Session) TogglePause() bool {
	next := !s.paused.Load()
	s.SetPaused(next)
	return next
}

// IsPaused reports whether the speech queue is currently paused.
func (s *Session) IsPaused() bool {
	return s.paused.Load()
}

// SkipCurrent aborts the currently playing TTS utterance and advances to the next item in the queue.
func (s *Session) SkipCurrent() {
	s.mu.RLock()
	p := s.player
	s.mu.RUnlock()
	if p != nil {
		p.Stop()
	}
}

// ClearQueue drains all pending items from the speech queue.
func (s *Session) ClearQueue() {
	for {
		select {
		case item := <-s.speechCh:
			s.updateLogStatus(item.entryID, StatusSkipped, "queue_cleared")
		default:
			return
		}
	}
}

// IsPanicked reports whether Emergency Panic Mute mode is currently latched active.
func (s *Session) IsPanicked() bool {
	return s.panicked.Load()
}

// SetPanicked latches or unlatches Emergency Panic Mute mode.
func (s *Session) SetPanicked(panicked bool) {
	s.panicked.Store(panicked)
	s.mu.Lock()
	p := s.player
	if panicked {
		s.lastSpeakerKey = ""
		s.lastSpokenAt = time.Time{}
		s.nowSpeaking = nil
	}
	s.mu.Unlock()

	if panicked {
		if p != nil {
			p.PanicStop()
		}
		s.ClearQueue()
		s.notifyNowSpeaking(nil)
	} else {
		if p != nil {
			p.ResetPanic()
		}
		select {
		case s.wakeCh <- struct{}{}:
		default:
		}
	}
}

// TogglePanic toggles Emergency Panic Mute mode on or off and returns the new panicked state.
func (s *Session) TogglePanic() bool {
	next := !s.panicked.Load()
	s.SetPanicked(next)
	return next
}

// PanicMute immediately halts audio output (<50ms), flushes the speech queue, and latches Panic Mute mode.
func (s *Session) PanicMute() {
	s.SetPanicked(true)
}

// ReplayLast re-enqueues the most recently spoken message for immediate playback.
func (s *Session) ReplayLast() bool {
	s.mu.RLock()
	last := s.lastSpoken
	s.mu.RUnlock()
	if last == nil {
		// Fall back to the latest entry in the log
		entries := s.LogSnapshot()
		if len(entries) == 0 {
			return false
		}
		last = &entries[len(entries)-1]
	}
	return s.ReplayEntry(last.ID)
}

// ReplayEntry re-enqueues a specific LogEntry by ID for playback.
func (s *Session) ReplayEntry(entryID uint64) bool {
	s.mu.RLock()
	var target *LogEntry
	for _, cp := range slices.Backward(s.logEntries) {
		if cp.ID == entryID {
			target = &cp
			break
		}
	}
	speed := s.cfg.Audio.DefaultSpeed
	s.mu.RUnlock()

	if target == nil {
		return false
	}
	spoken := target.SpokenText
	if strings.TrimSpace(spoken) == "" {
		spoken = target.UserName + ": " + target.Text
	}
	s.updateLogStatus(target.ID, StatusQueued, "")
	select {
	case s.speechCh <- speechItem{
		entryID:    target.ID,
		slotID:     target.SlotID,
		spokenText: spoken,
		voiceID:    target.VoiceID,
		speed:      speed,
		volume:     target.Volume,
		pan:        target.Pan,
		isPriority: true,
		enqueuedAt: time.Now(),
	}:
		return true
	default:
		return false
	}
}

// MuteChatter adds a chatter username to Identity.IgnoredChatters and persists config.
func (s *Session) MuteChatter(username string) {
	clean := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	if clean == "" {
		return
	}
	_ = s.UpdateConfig(func(cfg *config.Config) {
		if !containsFold(cfg.Identity.IgnoredChatters, clean) {
			cfg.Identity.IgnoredChatters = append(cfg.Identity.IgnoredChatters, clean)
		}
	})
}

// TogglePriorityFriend toggles a chatter in Identity.PriorityChatters and returns true if added.
func (s *Session) TogglePriorityFriend(username string) bool {
	clean := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	if clean == "" {
		return false
	}
	added := false
	_ = s.UpdateConfig(func(cfg *config.Config) {
		if containsFold(cfg.Identity.PriorityChatters, clean) {
			cfg.Identity.PriorityChatters = slices.DeleteFunc(cfg.Identity.PriorityChatters, func(u string) bool {
				return strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(u), "@"), clean)
			})
			added = false
		} else {
			cfg.Identity.PriorityChatters = append(cfg.Identity.PriorityChatters, clean)
			added = true
		}
	})
	return added
}

// AssignChatterVoice assigns a specific neural TTS voice ID to a chatter username.
func (s *Session) AssignChatterVoice(username, voiceID string) {
	clean := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	if clean == "" {
		return
	}
	_ = s.UpdateConfig(func(cfg *config.Config) {
		if cfg.Identity.ChatterVoices == nil {
			cfg.Identity.ChatterVoices = make(map[string]string)
		}
		if strings.TrimSpace(voiceID) == "" {
			delete(cfg.Identity.ChatterVoices, clean)
		} else {
			cfg.Identity.ChatterVoices[clean] = strings.TrimSpace(voiceID)
		}
	})
}

// PreviewVoice synthesizes and plays a short sample phrase using the specified voiceID.
func (s *Session) PreviewVoice(voiceID string, sampleText string) {
	s.mu.RLock()
	eng := s.engine
	p := s.player
	speed := s.cfg.Audio.DefaultSpeed
	defVoice := s.cfg.Audio.DefaultVoice
	s.mu.RUnlock()

	if eng == nil || p == nil {
		return
	}
	vid := strings.TrimSpace(voiceID)
	if vid == "" {
		vid = defVoice
	}
	if strings.TrimSpace(sampleText) == "" {
		sampleText = "Hello from Hearsay! Ready to read live chat aloud."
	}
	if speed <= 0 {
		speed = 1.0
	}
	go func() {
		samples, sr, err := eng.Synthesize(sampleText, vid, float32(speed))
		if err == nil && len(samples) > 0 {
			_ = p.PlayWithOptions(samples, sr, audio.PlayOptions{Gain: 1.0, Pan: 0})
		}
	}()
}

// ReloadEngine ensures all enabled TTS model families in Config are downloaded to modelDir
// and initializes a new MultiEngine, replacing the active TTS engine.
func (s *Session) ReloadEngine(modelDir string, onProgress func(modelName string, pct float64)) error {
	if strings.TrimSpace(modelDir) == "" {
		modelDir = tts.DefaultModelDir()
	}
	cfg := s.ConfigSnapshot()
	enabledModels := cfg.Audio.EffectiveEnabledModels()

	for _, fam := range enabledModels {
		if fam == tts.ModelKokoro || fam == tts.ModelKokoroIntl {
			if !tts.IsKokoroInstalledForMode(modelDir, cfg.Audio.UseGPU) {
				if err := tts.DownloadKokoroModelForMode(modelDir, cfg.Audio.UseGPU, func(cur, total int64) {
					if onProgress != nil && total > 0 {
						onProgress("Kokoro-82M", float64(cur)/float64(total))
					}
				}); err != nil {
					return err
				}
			}
			continue
		}
		if !tts.IsModelFamilyInstalled(modelDir, fam, cfg.Audio.UseGPU) {
			spec, _ := tts.ModelSpecByID(fam)
			name := spec.ShortName
			if name == "" {
				name = fam
			}
			if err := tts.DownloadModelFamily(modelDir, fam, cfg.Audio.UseGPU, func(cur, total int64) {
				if onProgress != nil && total > 0 {
					onProgress(name, float64(cur)/float64(total))
				}
			}); err != nil {
				return err
			}
		}
	}

	threads := cfg.Audio.NumThreads
	if threads <= 0 {
		threads = 4
	}
	newEng, err := tts.NewMultiEngine(modelDir, enabledModels, threads, cfg.Audio.UseGPU, tts.DefaultMaxClauseWords, 0)
	if err != nil {
		return err
	}

	s.mu.Lock()
	oldEng := s.engine
	s.engine = newEng
	s.mu.Unlock()

	if oldEng != nil {
		_ = oldEng.Close()
	}
	return nil
}

// UpdateModelFamilies force-updates the specified TTS model families on disk and reloads the active MultiEngine.
func (s *Session) UpdateModelFamilies(modelDir string, families []string, onProgress func(modelName string, pct float64)) error {
	if strings.TrimSpace(modelDir) == "" {
		modelDir = tts.DefaultModelDir()
	}
	cfg := s.ConfigSnapshot()
	enabledModels := cfg.Audio.EffectiveEnabledModels()

	// Detach and close the active engine first so open ONNX files can be overwritten on Windows
	s.mu.Lock()
	oldEng := s.engine
	s.engine = nil
	s.mu.Unlock()
	if oldEng != nil {
		_ = oldEng.Close()
	}
	tts.ReclaimOSMemory()

	for _, fam := range families {
		spec, _ := tts.ModelSpecByID(fam)
		name := spec.ShortName
		if name == "" {
			name = fam
		}
		if err := tts.ForceUpdateModelFamily(modelDir, fam, cfg.Audio.UseGPU, func(cur, total int64) {
			if onProgress != nil && total > 0 {
				onProgress(name, float64(cur)/float64(total))
			}
		}); err != nil {
			return err
		}
	}

	threads := cfg.Audio.NumThreads
	if threads <= 0 {
		threads = 4
	}
	newEng, err := tts.NewMultiEngine(modelDir, enabledModels, threads, cfg.Audio.UseGPU, tts.DefaultMaxClauseWords, 0)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.engine = newEng
	s.mu.Unlock()
	return nil
}

func (s *Session) notifyLogUpdated() {
	s.mu.RLock()
	fn := s.onLogUpdated
	s.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (s *Session) notifySlotsUpdated() {
	s.mu.RLock()
	fn := s.onSlotsUpdated
	s.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (s *Session) notifyNowSpeaking(entry *LogEntry) {
	s.mu.RLock()
	fn := s.onNowSpeaking
	s.mu.RUnlock()
	if fn != nil {
		fn(entry)
	}
}

// Close stops all active connectors and terminates the speech worker.
func (s *Session) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	close(s.workerStopCh)

	s.mu.Lock()
	conns := make([]SlotConnector, 0, len(s.slots))
	for _, as := range s.slots {
		if as.connector != nil {
			conns = append(conns, as.connector)
		}
	}
	s.slots = make(map[string]*activeSlot)
	s.slotOrder = nil
	p := s.player
	s.mu.Unlock()

	if p != nil {
		p.Stop()
	}
	for _, c := range conns {
		_ = c.Stop()
	}
	<-s.workerStopped
	return nil
}
