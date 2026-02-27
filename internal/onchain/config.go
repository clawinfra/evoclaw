package onchain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const chainsFileName = "chains.json"

// chainsFile is the default path for the chains config (~/.evoclaw/chains.json).
func chainsFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".evoclaw", chainsFileName)
}

// ChainsConfig is the top-level structure persisted to chains.json.
type ChainsConfig struct {
	Chains map[string]ChainConfig `json:"chains"`
}

// chainsStore is the singleton in-memory store backed by chains.json.
type chainsStore struct {
	mu   sync.RWMutex
	path string
	cfg  ChainsConfig
}

var defaultStore *chainsStore

// DefaultChainsStore returns the singleton store backed by ~/.evoclaw/chains.json.
func DefaultChainsStore() *chainsStore {
	if defaultStore == nil {
		defaultStore = &chainsStore{path: chainsFilePath()}
	}
	return defaultStore
}

// NewChainsStore creates a store backed by the given file path.
// Useful for testing or custom data directories.
func NewChainsStore(path string) *chainsStore {
	return &chainsStore{path: path}
}

// Load reads the chains config from disk into memory.
// If the file does not exist, an empty config is initialised (not an error).
func (s *chainsStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.cfg = ChainsConfig{Chains: make(map[string]ChainConfig)}
			return nil
		}
		return fmt.Errorf("read chains config: %w", err)
	}

	var cfg ChainsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse chains config: %w", err)
	}
	if cfg.Chains == nil {
		cfg.Chains = make(map[string]ChainConfig)
	}
	s.cfg = cfg
	return nil
}

// Save writes the current in-memory config to disk atomically.
func (s *chainsStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal chains config: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write chains config: %w", err)
	}
	return os.Rename(tmp, s.path)
}

// Add inserts or replaces a chain config entry.
func (s *chainsStore) Add(id string, cfg ChainConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.Chains == nil {
		s.cfg.Chains = make(map[string]ChainConfig)
	}
	s.cfg.Chains[id] = cfg
}

// Remove deletes a chain config entry by ID.
func (s *chainsStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cfg.Chains[id]; !ok {
		return fmt.Errorf("chain %q not found", id)
	}
	delete(s.cfg.Chains, id)
	return nil
}

// Get returns the ChainConfig for a given ID.
func (s *chainsStore) Get(id string) (ChainConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.cfg.Chains[id]
	return cfg, ok
}

// List returns a copy of all stored chain configs keyed by ID.
func (s *chainsStore) List() map[string]ChainConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]ChainConfig, len(s.cfg.Chains))
	for k, v := range s.cfg.Chains {
		out[k] = v
	}
	return out
}
