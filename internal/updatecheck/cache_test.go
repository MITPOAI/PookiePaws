package updatecheck

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(filepath.Join(dir, "uc.json"))

	entry := &CacheEntry{
		CheckedAt: time.Now().UTC(),
		Release:   Release{TagName: "v0.6.0", HTMLURL: "https://x"},
	}
	if err := c.Save(entry); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := c.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Release.TagName != "v0.6.0" {
		t.Errorf("TagName = %q", got.Release.TagName)
	}
}

func TestCacheLoadMissingFile(t *testing.T) {
	c := NewCache(filepath.Join(t.TempDir(), "missing.json"))
	got, err := c.Load()
	if err != nil {
		t.Fatalf("Load on missing file should not error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil entry, got %+v", got)
	}
}

func TestCacheLoadCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "uc.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := NewCache(path)
	got, err := c.Load()
	if err != nil {
		t.Fatalf("Load on corrupt file should not error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil entry on corrupt file, got %+v", got)
	}
}

func TestCacheIsExpired(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name      string
		checkedAt time.Time
		ttl       time.Duration
		want      bool
	}{
		{"fresh", now.Add(-1 * time.Hour), 24 * time.Hour, false},
		{"expired", now.Add(-25 * time.Hour), 24 * time.Hour, true},
		{"exactly at TTL", now.Add(-24 * time.Hour), 24 * time.Hour, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &CacheEntry{CheckedAt: tc.checkedAt}
			if got := e.IsExpired(tc.ttl, now); got != tc.want {
				t.Fatalf("IsExpired = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCacheAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "uc.json")
	c := NewCache(path)
	if err := c.Save(&CacheEntry{CheckedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("temp file leaked: %s", e.Name())
		}
	}
}
