package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// ErrProductionConfigInTest is returned when code attempts to resolve production
// configuration locations during unit test execution.
var ErrProductionConfigInTest = errors.New("production config locations are forbidden during tests; use NewMemoryStore() or TestLocations(t.TempDir())")

// Locations encapsulates the resolved file paths for Hearsay configuration.
type Locations struct {
	ConfigDir  string
	ConfigFile string
	isTest     bool
}

// ProductionLocations resolves the standard OS user config directory for Hearsay.
// Fails immediately if called inside `go test`.
func ProductionLocations() (Locations, error) {
	if testing.Testing() {
		return Locations{}, ErrProductionConfigInTest
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return Locations{}, fmt.Errorf("resolve user config dir: %w", err)
	}
	dir := filepath.Join(base, AppID)
	return Locations{
		ConfigDir:  dir,
		ConfigFile: filepath.Join(dir, "config.json"),
	}, nil
}

// TestLocations creates an isolated Locations struct inside a temporary test directory.
func TestLocations(tempDir string) Locations {
	dir := filepath.Join(tempDir, AppID)
	return Locations{
		ConfigDir:  dir,
		ConfigFile: filepath.Join(dir, "config.json"),
		isTest:     true,
	}
}

// Store abstracts configuration persistence for production files and isolated tests.
type Store interface {
	Load() (*Config, error)
	Save(cfg *Config) error
	Location() string
}

// FileStore persists configuration to disk at a resolved Locations path.
type FileStore struct {
	mu  sync.Mutex
	loc Locations
}

// NewFileStore creates a disk-backed configuration store.
func NewFileStore(loc Locations) (*FileStore, error) {
	if testing.Testing() && !loc.isTest {
		return nil, ErrProductionConfigInTest
	}
	if loc.ConfigFile == "" {
		return nil, errors.New("config file path cannot be empty")
	}
	return &FileStore{loc: loc}, nil
}

// Load reads the configuration from disk.
func (s *FileStore) Load() (*Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Load(s.loc.ConfigFile)
}

// Save writes the configuration atomically to disk.
func (s *FileStore) Save(cfg *Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Save(s.loc.ConfigFile, cfg)
}

// Location returns the file path managed by this store.
func (s *FileStore) Location() string {
	return s.loc.ConfigFile
}

// MemoryStore is an in-memory Store implementation for unit tests.
type MemoryStore struct {
	mu  sync.Mutex
	cfg *Config
}

// NewMemoryStore creates an isolated in-memory configuration store initialized with defaults or the provided config.
func NewMemoryStore(initial ...*Config) *MemoryStore {
	var cfg *Config
	if len(initial) > 0 && initial[0] != nil {
		cfg = initial[0].Clone()
	} else {
		cfg = DefaultConfig()
	}
	return &MemoryStore{cfg: cfg}
}

// Load returns a copy of the in-memory configuration.
func (m *MemoryStore) Load() (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Clone(), nil
}

// Save updates the in-memory configuration.
func (m *MemoryStore) Save(cfg *Config) error {
	if cfg == nil {
		return errors.New("cannot save nil config")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg.Clone()
	return nil
}

// Location returns a synthetic identifier for the memory store.
func (m *MemoryStore) Location() string {
	return "memory://config.json"
}
