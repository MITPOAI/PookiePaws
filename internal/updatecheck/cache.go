package updatecheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CacheEntry is a single cached lookup result.
type CacheEntry struct {
	CheckedAt time.Time `json:"checked_at"`
	Release   Release   `json:"release"`
}

// IsExpired reports whether the entry is older than ttl relative to now.
// An entry exactly at the TTL boundary is considered expired (>= ttl).
func (e *CacheEntry) IsExpired(ttl time.Duration, now time.Time) bool {
	return now.Sub(e.CheckedAt) >= ttl
}

// Cache persists a single CacheEntry to disk as JSON.
type Cache struct {
	path string
}

// NewCache builds a Cache writing to the given file path.
func NewCache(path string) *Cache {
	return &Cache{path: path}
}

// DefaultCachePath returns the conventional location under the user cache dir.
// Falls back to a tempdir-relative path if UserCacheDir fails.
func DefaultCachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "pookiepaws", "update-check.json")
}

// Load returns the persisted entry or (nil, nil) if the file is missing or
// corrupt. A corrupt file is treated as "no cache" — the caller will refetch.
func (c *Cache) Load() (*CacheEntry, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read cache: %w", err)
	}
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		// Corrupt cache is non-fatal; force a refetch.
		return nil, nil
	}
	return &entry, nil
}

// Save writes the entry atomically: write to <path>.tmp, then rename.
func (c *Cache) Save(entry *CacheEntry) error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return fmt.Errorf("mkdir cache: %w", err)
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	// Process-unique tmp suffix so two concurrent `pookie` processes can't
	// race on the same staging path (one's WriteFile would clobber the other's,
	// and the cleanup os.Remove could nuke an in-flight tmp belonging to a peer).
	tmp := fmt.Sprintf("%s.%d.tmp", c.path, os.Getpid())
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp cache: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename cache: %w", err)
	}
	return nil
}
