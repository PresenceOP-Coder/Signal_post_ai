package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"signalpost/internal/model"
)

// Store manages persistence of company profiles and baseline seed data.
type Store struct {
	mu       sync.RWMutex
	storeDir string
	seedDir  string
}

// NewStore initializes a profile store pointing to the output directory and optional seed directory.
func NewStore(storeDir, seedDir string) (*Store, error) {
	if storeDir == "" {
		storeDir = "profiles"
	}
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store dir %q: %w", storeDir, err)
	}
	return &Store{
		storeDir: storeDir,
		seedDir:  seedDir,
	}, nil
}

// LoadProfile attempts to read an existing profile from storeDir first, then falling back to seedDir.
func (s *Store) LoadProfile(orgnr string) (*model.Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Check primary store directory
	primaryPath := filepath.Join(s.storeDir, orgnr+".json")
	if data, err := os.ReadFile(primaryPath); err == nil {
		var p model.Profile
		if err := json.Unmarshal(data, &p); err == nil {
			return &p, nil
		}
	}

	// 2. Check seed directory if configured
	if s.seedDir != "" {
		seedPath := filepath.Join(s.seedDir, orgnr+".json")
		if data, err := os.ReadFile(seedPath); err == nil {
			var p model.Profile
			if err := json.Unmarshal(data, &p); err == nil {
				return &p, nil
			}
		}
	}

	// Not found
	return nil, nil
}

// SaveProfile writes a single profile to profiles/{orgnr}.json.
func (s *Store) SaveProfile(p *model.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal profile %s: %w", p.Orgnr, err)
	}

	targetPath := filepath.Join(s.storeDir, p.Orgnr+".json")
	return os.WriteFile(targetPath, data, 0644)
}

// WriteJSONL appends or creates the aggregate profiles.jsonl file.
func (s *Store) WriteJSONL(filePath string, profiles []*model.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", filePath, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, p := range profiles {
		if err := enc.Encode(p); err != nil {
			return fmt.Errorf("failed to encode profile %s: %w", p.Orgnr, err)
		}
	}
	return nil
}
