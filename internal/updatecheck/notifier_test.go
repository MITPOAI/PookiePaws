package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestShouldSkipCI(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("POOKIEPAWS_NO_UPDATE_NOTIFIER", "")
	if !ShouldSkip() {
		t.Fatal("expected skip when CI is set")
	}
}

func TestShouldSkipOptOut(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("POOKIEPAWS_NO_UPDATE_NOTIFIER", "1")
	if !ShouldSkip() {
		t.Fatal("expected skip when POOKIEPAWS_NO_UPDATE_NOTIFIER=1")
	}
}

func TestShouldSkipNeitherSet(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("POOKIEPAWS_NO_UPDATE_NOTIFIER", "")
	if ShouldSkip() {
		t.Fatal("expected no skip when neither env is set")
	}
}

func TestCheckUsesCache(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(filepath.Join(dir, "uc.json"))
	_ = cache.Save(&CacheEntry{
		CheckedAt: time.Now().UTC(),
		Release:   Release{TagName: "v0.6.0", HTMLURL: "https://x"},
	})

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(500)
	}))
	defer srv.Close()

	notice, err := Check(context.Background(), Options{
		CurrentVersion: "0.5.2",
		BaseURL:        srv.URL,
		Cache:          cache,
		TTL:            24 * time.Hour,
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if called {
		t.Fatal("HTTP server should not be called when cache is fresh")
	}
	if notice == nil || notice.Latest != "v0.6.0" {
		t.Fatalf("notice = %+v", notice)
	}
}

func TestCheckRefetchesWhenForced(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(filepath.Join(dir, "uc.json"))
	_ = cache.Save(&CacheEntry{
		CheckedAt: time.Now().UTC(),
		Release:   Release{TagName: "v0.6.0"},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.7.0","html_url":"https://x"}`))
	}))
	defer srv.Close()

	notice, err := Check(context.Background(), Options{
		CurrentVersion: "0.5.2",
		BaseURL:        srv.URL,
		Cache:          cache,
		TTL:            24 * time.Hour,
		Timeout:        time.Second,
		Force:          true,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if notice.Latest != "v0.7.0" {
		t.Fatalf("expected v0.7.0 after force, got %s", notice.Latest)
	}
}

func TestCheckReturnsNilWhenUpToDate(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(filepath.Join(dir, "uc.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.5.2","html_url":"https://x"}`))
	}))
	defer srv.Close()

	notice, err := Check(context.Background(), Options{
		CurrentVersion: "0.5.2",
		BaseURL:        srv.URL,
		Cache:          cache,
		TTL:            24 * time.Hour,
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if notice != nil {
		t.Fatalf("expected nil notice when up to date, got %+v", notice)
	}
}

func TestUpgradeHintFallback(t *testing.T) {
	// With an empty PATH, both winget and brew lookups fail; we expect the
	// install-script fallback hint.
	t.Setenv("PATH", "")
	hint := UpgradeHint(runtime.GOOS)
	if hint == "" {
		t.Fatal("expected non-empty fallback hint")
	}
}
