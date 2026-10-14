package fetch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CacheEntry represents the cached HTTP response stored on disk.
type CacheEntry struct {
	StatusCode int       `json:"status_code"`
	Body       []byte    `json:"body"`
	FinalURL   string    `json:"final_url"`
	FetchedAt  time.Time `json:"fetched_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// DiskCache provides a file-system based cache for HTTP responses.
type DiskCache struct {
	dir string
}

// NewDiskCache creates a new disk cache in the specified directory.
func NewDiskCache(dir string) (*DiskCache, error) {
	if dir == "" {
		dir = filepath.Join(".cache", "http")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	return &DiskCache{dir: dir}, nil
}

func (c *DiskCache) keyToPath(key string) string {
	h := sha256.Sum256([]byte(key))
	hashStr := hex.EncodeToString(h[:])
	return filepath.Join(c.dir, hashStr+".json")
}

// Get retrieves an unexpired cache entry for the key, if present.
func (c *DiskCache) Get(key string) (*CacheEntry, bool) {
	if c == nil {
		return nil, false
	}
	path := c.keyToPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	if time.Now().UTC().After(entry.ExpiresAt) {
		_ = os.Remove(path) // Evict expired entry
		return nil, false
	}

	return &entry, true
}

// Put saves a response to disk with a defined TTL.
func (c *DiskCache) Put(key string, statusCode int, body []byte, finalURL string, fetchedAt time.Time, ttl time.Duration) error {
	if c == nil {
		return nil
	}
	entry := CacheEntry{
		StatusCode: statusCode,
		Body:       body,
		FinalURL:   finalURL,
		FetchedAt:  fetchedAt,
		ExpiresAt:  fetchedAt.Add(ttl),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(c.keyToPath(key), data, 0644)
}
