package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/updatecheck"
)

func TestRunVersionPrintsBasic(t *testing.T) {
	var out bytes.Buffer
	err := runVersion(context.Background(), versionConfig{
		Version:   "0.5.2",
		Stdout:    &out,
		Stderr:    &bytes.Buffer{},
		Check:     false,
		CachePath: filepath.Join(t.TempDir(), "uc.json"),
	})
	if err != nil {
		t.Fatalf("runVersion: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "pookie v0.5.2") {
		t.Errorf("output missing version: %q", got)
	}
	if !strings.Contains(got, runtime.GOOS) {
		t.Errorf("output missing GOOS: %q", got)
	}
}

func TestRunVersionCheckShowsLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.6.0","html_url":"https://example/r"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	err := runVersion(context.Background(), versionConfig{
		Version:   "0.5.2",
		Stdout:    &stdout,
		Stderr:    &stderr,
		Check:     true,
		CachePath: filepath.Join(t.TempDir(), "uc.json"),
		BaseURL:   srv.URL,
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("runVersion: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "latest:  v0.6.0") {
		t.Errorf("expected latest line: %q", out)
	}
	if !strings.Contains(out, "https://example/r") {
		t.Errorf("expected release URL: %q", out)
	}
}

func TestRunVersionCheckUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.5.2"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	err := runVersion(context.Background(), versionConfig{
		Version:   "0.5.2",
		Stdout:    &stdout,
		Stderr:    &stderr,
		Check:     true,
		CachePath: filepath.Join(t.TempDir(), "uc.json"),
		BaseURL:   srv.URL,
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("runVersion: %v", err)
	}
	if !strings.Contains(stdout.String(), "up to date") {
		t.Errorf("expected up-to-date message: %q", stdout.String())
	}
}

func TestRunVersionPrintsCachedHintWithoutCheck(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "uc.json")
	cache := updatecheck.NewCache(cachePath)
	if err := cache.Save(&updatecheck.CacheEntry{
		CheckedAt: time.Now().UTC(),
		Release:   updatecheck.Release{TagName: "v9.9.9", HTMLURL: "https://example/r"},
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := runVersion(context.Background(), versionConfig{
		Version:   "0.5.2",
		Stdout:    &stdout,
		Stderr:    &stderr,
		Check:     false,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("runVersion: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "latest:  v9.9.9") {
		t.Errorf("expected cached latest line on stdout: %q", got)
	}
	if !strings.Contains(got, "https://example/r") {
		t.Errorf("expected cached release URL on stdout: %q", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected nothing on stderr, got %q", stderr.String())
	}
}

func TestRunVersionSilentWhenCacheMissing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runVersion(context.Background(), versionConfig{
		Version:   "0.5.2",
		Stdout:    &stdout,
		Stderr:    &stderr,
		Check:     false,
		CachePath: filepath.Join(t.TempDir(), "missing.json"),
	})
	if err != nil {
		t.Fatalf("runVersion: %v", err)
	}
	got := stdout.String()
	if strings.Contains(got, "latest:") || strings.Contains(got, "upgrade:") {
		t.Errorf("expected no upgrade hint when cache missing, got %q", got)
	}
}

func TestRunVersionSilentWhenCacheExpired(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "uc.json")
	cache := updatecheck.NewCache(cachePath)
	if err := cache.Save(&updatecheck.CacheEntry{
		CheckedAt: time.Now().UTC().Add(-72 * time.Hour),
		Release:   updatecheck.Release{TagName: "v9.9.9", HTMLURL: "https://example/r"},
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := runVersion(context.Background(), versionConfig{
		Version:   "0.5.2",
		Stdout:    &stdout,
		Stderr:    &stderr,
		Check:     false,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("runVersion: %v", err)
	}
	got := stdout.String()
	if strings.Contains(got, "latest:") || strings.Contains(got, "upgrade:") {
		t.Errorf("expected no upgrade hint when cache expired, got %q", got)
	}
}

func TestVersionConfigDefaults(t *testing.T) {
	// Sanity: zero-value CachePath must be filled by the caller wrapper, not by
	// runVersion. We assert that an empty CachePath produces an error.
	err := runVersion(context.Background(), versionConfig{
		Version: "0.5.2",
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
		Check:   true,
	})
	if err == nil {
		t.Fatal("expected error when CachePath is empty and Check is true")
	}
}

// Smoke: the package-level shim that the dispatcher calls is wired correctly.
var _ = updatecheck.DefaultCachePath
