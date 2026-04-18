# Update Notifier + `pookie version --check` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a cached GitHub-Releases-backed update notifier and a `pookie version [--check]` subcommand that surfaces a newer release on `stderr` during interactive runs, without ever auto-installing anything.

**Architecture:** A new `internal/updatecheck` package owns release fetching, semver comparison, and a 24-hour file cache under the user cache dir. The CLI gains a `version` subcommand with a `--check` flag that forces a live lookup; all other interactive commands fire a non-blocking background check that prints a one-line notice on `stderr` after the command completes. Opt-outs: `CI=*` and `POOKIEPAWS_NO_UPDATE_NOTIFIER=1`. Upgrades stay external — hints prefer `winget`/`brew` when on `PATH`, otherwise reference `install.{ps1,sh}`.

**Tech Stack:** Go 1.22, standard library only (`net/http`, `encoding/json`, `os`, `path/filepath`, `time`), `golang.org/x/mod/semver` (already a transitive dep of most Go modules — verify and add if needed).

---

## File Structure

| Path | Responsibility |
|------|----------------|
| `internal/updatecheck/release.go` | NEW — `Release` type, `FetchLatest(ctx, repo)` GitHub API call, parsing |
| `internal/updatecheck/semver.go` | NEW — `IsNewer(current, latest string) bool`, version normalization (`v0.5.2` ↔ `0.5.2`) |
| `internal/updatecheck/cache.go` | NEW — `Load`/`Save` cached release with TTL, atomic write, lives under `os.UserCacheDir()/pookiepaws/update-check.json` |
| `internal/updatecheck/notifier.go` | NEW — `Check(ctx, current, opts) (*Notice, error)` orchestrates cache + fetch; `ShouldSkip()` checks env vars; `UpgradeHint()` chooses winget/brew/script |
| `internal/updatecheck/release_test.go` | NEW — release JSON parsing, HTTP timeout, error paths |
| `internal/updatecheck/semver_test.go` | NEW — newer/older/equal/prerelease/invalid cases |
| `internal/updatecheck/cache_test.go` | NEW — TTL hit/miss, missing file, corrupted file, atomic write |
| `internal/updatecheck/notifier_test.go` | NEW — opt-out env, hint selection, end-to-end via stub HTTP server |
| `cmd/pookie/version.go` | NEW — `cmdVersion(args)` handler with `--check` flag, prints installed/latest/URL/hint |
| `cmd/pookie/version_test.go` | NEW — argument parsing, output format |
| `cmd/pookie/main.go` | MODIFY — replace inline `case "version", "--version", "-v": printVersion()` with `cmdVersion(os.Args[2:])`; remove unused `printVersion` if nothing else calls it |
| `cmd/pookie/notifier.go` | NEW — small helper `maybeShowUpdateNotice(ctx, current)` invoked at end of interactive commands; isolates env detection |
| `go.mod` / `go.sum` | MODIFY if needed — add `golang.org/x/mod` for `semver` |

---

## Constants and Repository Identity

The GitHub repo is `MITPOAI/PookiePaws` (per `git remote -v`). The current version string lives at `cmd/pookie/main.go:21` (`var version = "0.5.2"`). The notifier package must NOT import `cmd/pookie`; the version is passed in by the caller.

The release URL template is `https://api.github.com/repos/%s/releases/latest`. The notifier must set `User-Agent: pookiepaws-update-check/<version>` (GitHub rejects requests without a UA) and `Accept: application/vnd.github+json`.

---

## Task 1: Add semver helper

**Files:**
- Create: `internal/updatecheck/semver.go`
- Test: `internal/updatecheck/semver_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/updatecheck/semver_test.go`:

```go
package updatecheck

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		name           string
		current, latest string
		want           bool
	}{
		{"latest is newer", "0.5.2", "0.5.3", true},
		{"latest is newer with v prefix", "v0.5.2", "v0.5.3", true},
		{"mixed prefixes", "0.5.2", "v0.6.0", true},
		{"latest is older", "0.6.0", "0.5.9", false},
		{"equal", "0.5.2", "0.5.2", false},
		{"latest is major bump", "0.5.2", "1.0.0", true},
		{"prerelease less than release", "1.0.0-rc.1", "1.0.0", true},
		{"release greater than prerelease", "1.0.0", "1.0.0-rc.1", false},
		{"invalid current returns false", "not-a-version", "1.0.0", false},
		{"invalid latest returns false", "1.0.0", "not-a-version", false},
		{"empty current returns false", "", "1.0.0", false},
		{"empty latest returns false", "1.0.0", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsNewer(tc.current, tc.latest)
			if got != tc.want {
				t.Fatalf("IsNewer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0.5.2", "v0.5.2"},
		{"v0.5.2", "v0.5.2"},
		{"  v1.0.0  ", "v1.0.0"},
		{"", ""},
		{"garbage", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := Normalize(tc.in)
			if got != tc.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/updatecheck/...`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Add `golang.org/x/mod` if missing**

Run: `go list -m golang.org/x/mod 2>&1`

If "not a known dependency", run: `go get golang.org/x/mod@latest`

- [ ] **Step 4: Implement `semver.go`**

Create `internal/updatecheck/semver.go`:

```go
// Package updatecheck checks GitHub releases for newer versions and surfaces
// a non-blocking notice on stderr. The package never installs anything.
package updatecheck

import (
	"strings"

	"golang.org/x/mod/semver"
)

// Normalize trims whitespace, ensures a leading "v", and returns "" if the
// result is not a valid semver string.
func Normalize(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return ""
	}
	return v
}

// IsNewer reports whether `latest` is strictly greater than `current` under
// semantic versioning. Invalid inputs always return false (fail-closed: never
// nag the user about a version we can't parse).
func IsNewer(current, latest string) bool {
	c, l := Normalize(current), Normalize(latest)
	if c == "" || l == "" {
		return false
	}
	return semver.Compare(l, c) > 0
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/updatecheck/... -run 'TestIsNewer|TestNormalize' -v`
Expected: PASS for all cases.

- [ ] **Step 6: Commit**

```bash
git add internal/updatecheck/semver.go internal/updatecheck/semver_test.go go.mod go.sum
git commit -m "feat(updatecheck): add semver normalization and comparison helpers"
```

---

## Task 2: Add release fetcher

**Files:**
- Create: `internal/updatecheck/release.go`
- Test: `internal/updatecheck/release_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/updatecheck/release_test.go`:

```go
package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchLatestSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); !strings.Contains(got, "pookiepaws-update-check") {
			t.Errorf("User-Agent = %q, want pookiepaws-update-check prefix", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.6.0","html_url":"https://example/release","name":"0.6.0","draft":false,"prerelease":false,"published_at":"2026-04-15T10:00:00Z"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "0.5.2", 2*time.Second)
	rel, err := c.FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if rel.TagName != "v0.6.0" {
		t.Errorf("TagName = %q", rel.TagName)
	}
	if rel.HTMLURL != "https://example/release" {
		t.Errorf("HTMLURL = %q", rel.HTMLURL)
	}
}

func TestFetchLatestSkipsDraftsAndPrereleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.6.0","draft":true,"prerelease":false}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "0.5.2", 2*time.Second)
	_, err := c.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for draft release")
	}
}

func TestFetchLatestNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"rate limit"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "0.5.2", 2*time.Second)
	_, err := c.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestFetchLatestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"tag_name":"v0.6.0"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "0.5.2", 50*time.Millisecond)
	_, err := c.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/updatecheck/... -run TestFetch -v`
Expected: FAIL — `NewClient`, `FetchLatest`, `Release` undefined.

- [ ] **Step 3: Implement `release.go`**

Create `internal/updatecheck/release.go`:

```go
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultRepo is the GitHub `owner/repo` identifier checked by the notifier.
const DefaultRepo = "MITPOAI/PookiePaws"

// DefaultBaseURL is the GitHub REST API base. Override in tests via NewClient.
const DefaultBaseURL = "https://api.github.com"

// Release is the trimmed subset of the GitHub release payload we care about.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

// Client fetches the latest release for a single repo.
type Client struct {
	baseURL    string
	repo       string
	userAgent  string
	httpClient *http.Client
}

// NewClient builds a Client with the given API base and timeout. `currentVersion`
// is embedded in the User-Agent so GitHub's logs identify the caller.
func NewClient(baseURL, currentVersion string, timeout time.Duration) *Client {
	return &Client{
		baseURL:   baseURL,
		repo:      DefaultRepo,
		userAgent: "pookiepaws-update-check/" + currentVersion,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// WithRepo overrides the default repository.
func (c *Client) WithRepo(repo string) *Client {
	c.repo = repo
	return c
}

// FetchLatest returns the latest non-draft, non-prerelease GitHub release.
// Drafts and prereleases are intentionally treated as "no release" — the
// notifier should never push users to unpublished or experimental builds.
func (c *Client) FetchLatest(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.baseURL, c.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github returned %d: %s", resp.StatusCode, string(body))
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	if rel.Draft || rel.Prerelease {
		return nil, fmt.Errorf("latest release is draft or prerelease (%s)", rel.TagName)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("release missing tag_name")
	}
	return &rel, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/updatecheck/... -run TestFetch -v`
Expected: PASS for all four subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/updatecheck/release.go internal/updatecheck/release_test.go
git commit -m "feat(updatecheck): add GitHub releases client with timeout and draft filtering"
```

---

## Task 3: Add cache layer

**Files:**
- Create: `internal/updatecheck/cache.go`
- Test: `internal/updatecheck/cache_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/updatecheck/cache_test.go`:

```go
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
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/updatecheck/... -run TestCache -v`
Expected: FAIL — undefined `NewCache`, `CacheEntry`.

- [ ] **Step 3: Implement `cache.go`**

Create `internal/updatecheck/cache.go`:

```go
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
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp cache: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename cache: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/updatecheck/... -run TestCache -v`
Expected: PASS for all five subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/updatecheck/cache.go internal/updatecheck/cache_test.go
git commit -m "feat(updatecheck): add 24h file cache with atomic writes"
```

---

## Task 4: Add notifier orchestration + opt-out + upgrade hints

**Files:**
- Create: `internal/updatecheck/notifier.go`
- Test: `internal/updatecheck/notifier_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/updatecheck/notifier_test.go`:

```go
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
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/updatecheck/... -run 'TestShouldSkip|TestCheck|TestUpgradeHint' -v`
Expected: FAIL — `Options`, `Notice`, `Check`, `ShouldSkip`, `UpgradeHint` undefined.

- [ ] **Step 3: Implement `notifier.go`**

Create `internal/updatecheck/notifier.go`:

```go
package updatecheck

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// Options configures a single Check call.
type Options struct {
	// CurrentVersion is the binary's compiled-in version (e.g. "0.5.2").
	CurrentVersion string
	// BaseURL overrides the GitHub API base; leave empty for production use.
	BaseURL string
	// Repo overrides the default `owner/repo`; leave empty for production use.
	Repo string
	// Cache stores lookup results across runs. Required.
	Cache *Cache
	// TTL is how long a cached entry stays fresh. Defaults to 24h.
	TTL time.Duration
	// Timeout caps the HTTP fetch. Defaults to 2s.
	Timeout time.Duration
	// Force bypasses the cache and always refetches.
	Force bool
}

// Notice describes a newer release the user could upgrade to.
type Notice struct {
	Current string
	Latest  string
	URL     string
	Hint    string
}

// ShouldSkip returns true when the user has opted out, either via the
// POOKIEPAWS_NO_UPDATE_NOTIFIER env var (matches gh/npm convention) or by
// running in CI (any non-empty `CI` value, the de facto industry signal).
func ShouldSkip() bool {
	if v := os.Getenv("POOKIEPAWS_NO_UPDATE_NOTIFIER"); v != "" && v != "0" && v != "false" {
		return true
	}
	if v := os.Getenv("CI"); v != "" && v != "0" && v != "false" {
		return true
	}
	return false
}

// Check returns a Notice when a newer release exists, or nil when the user is
// already on the latest version. Errors are returned for the caller to log;
// callers in the interactive notice path should treat any error as "no notice."
func Check(ctx context.Context, opts Options) (*Notice, error) {
	if opts.TTL == 0 {
		opts.TTL = 24 * time.Hour
	}
	if opts.Timeout == 0 {
		opts.Timeout = 2 * time.Second
	}
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if opts.Cache == nil {
		return nil, fmt.Errorf("updatecheck.Check: Cache is required")
	}

	var rel *Release
	if !opts.Force {
		entry, _ := opts.Cache.Load()
		if entry != nil && !entry.IsExpired(opts.TTL, time.Now().UTC()) {
			r := entry.Release
			rel = &r
		}
	}
	if rel == nil {
		client := NewClient(opts.BaseURL, opts.CurrentVersion, opts.Timeout)
		if opts.Repo != "" {
			client = client.WithRepo(opts.Repo)
		}
		fetched, err := client.FetchLatest(ctx)
		if err != nil {
			return nil, err
		}
		_ = opts.Cache.Save(&CacheEntry{
			CheckedAt: time.Now().UTC(),
			Release:   *fetched,
		})
		rel = fetched
	}

	if !IsNewer(opts.CurrentVersion, rel.TagName) {
		return nil, nil
	}
	return &Notice{
		Current: opts.CurrentVersion,
		Latest:  rel.TagName,
		URL:     rel.HTMLURL,
		Hint:    UpgradeHint(runtime.GOOS),
	}, nil
}

// UpgradeHint returns a one-line upgrade suggestion. winget is preferred on
// Windows, brew on macOS/Linux when present; otherwise the install-script
// fallback so users on bare systems still see a path forward.
func UpgradeHint(goos string) string {
	switch goos {
	case "windows":
		if _, err := exec.LookPath("winget"); err == nil {
			return "winget upgrade MITPOAI.PookiePaws"
		}
		return "Run install.ps1 from https://github.com/" + DefaultRepo
	default:
		if _, err := exec.LookPath("brew"); err == nil {
			return "brew upgrade mitpoai/pookiepaws/pookie"
		}
		return "Run install.sh from https://github.com/" + DefaultRepo
	}
}

// FormatNotice renders a one-line message suitable for stderr.
func (n *Notice) FormatNotice() string {
	return fmt.Sprintf("update available: %s → %s  (%s)\n  upgrade: %s",
		n.Current, n.Latest, n.URL, n.Hint)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/updatecheck/... -v`
Expected: PASS for all updatecheck tests.

- [ ] **Step 5: Commit**

```bash
git add internal/updatecheck/notifier.go internal/updatecheck/notifier_test.go
git commit -m "feat(updatecheck): add Check orchestrator with opt-out and upgrade hints"
```

---

## Task 5: Wire `pookie version [--check]` subcommand

**Files:**
- Create: `cmd/pookie/version.go`
- Test: `cmd/pookie/version_test.go`
- Modify: `cmd/pookie/main.go` (replace inline version handler)

- [ ] **Step 1: Write the failing test**

Create `cmd/pookie/version_test.go`:

```go
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
		Version:    "0.5.2",
		Stdout:     &out,
		Stderr:     &bytes.Buffer{},
		Check:      false,
		CachePath:  filepath.Join(t.TempDir(), "uc.json"),
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
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./cmd/pookie/... -run TestRunVersion -v`
Expected: FAIL — `runVersion`, `versionConfig` undefined.

- [ ] **Step 3: Implement `version.go`**

Create `cmd/pookie/version.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/mitpoai/pookiepaws/internal/updatecheck"
)

type versionConfig struct {
	Version   string
	Stdout    io.Writer
	Stderr    io.Writer
	Check     bool
	CachePath string
	BaseURL   string
	Timeout   time.Duration
}

func cmdVersion(args []string) {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	check := fs.Bool("check", false, "Force a live check against GitHub Releases")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	cfg := versionConfig{
		Version:   version,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Check:     *check,
		CachePath: updatecheck.DefaultCachePath(),
		Timeout:   3 * time.Second,
	}
	if err := runVersion(context.Background(), cfg); err != nil {
		fmt.Fprintf(cfg.Stderr, "version check failed: %v\n", err)
		os.Exit(1)
	}
}

func runVersion(ctx context.Context, cfg versionConfig) error {
	fmt.Fprintf(cfg.Stdout, "pookie v%s %s/%s %s\n",
		cfg.Version, runtime.GOOS, runtime.GOARCH, runtime.Version())

	if !cfg.Check {
		return nil
	}
	if cfg.CachePath == "" {
		return fmt.Errorf("CachePath required for --check")
	}

	notice, err := updatecheck.Check(ctx, updatecheck.Options{
		CurrentVersion: cfg.Version,
		BaseURL:        cfg.BaseURL,
		Cache:          updatecheck.NewCache(cfg.CachePath),
		Timeout:        cfg.Timeout,
		Force:          true,
	})
	if err != nil {
		return err
	}
	if notice == nil {
		fmt.Fprintln(cfg.Stdout, "up to date")
		return nil
	}
	fmt.Fprintf(cfg.Stdout, "latest:  %s\n  url:   %s\n  upgrade: %s\n",
		notice.Latest, notice.URL, notice.Hint)
	return nil
}
```

- [ ] **Step 4: Modify `cmd/pookie/main.go` to use the new dispatcher**

In `cmd/pookie/main.go`, replace the existing version case (currently lines 59–60):

```go
		case "version", "--version", "-v":
			printVersion()
```

with:

```go
		case "version", "--version", "-v":
			cmdVersion(os.Args[2:])
```

Also delete the now-unused `printVersion` function (currently at lines 70–72) — `runVersion` replaces it.

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 6: Run tests**

Run: `go test ./cmd/pookie/... -run TestRunVersion -v && go test ./internal/updatecheck/...`
Expected: PASS.

- [ ] **Step 7: Manual smoke**

Run: `go run ./cmd/pookie version`
Expected: `pookie v0.5.2 <os>/<arch> go1.22.x`

Run: `go run ./cmd/pookie version --check`
Expected: live output including `latest:` or `up to date`.

- [ ] **Step 8: Commit**

```bash
git add cmd/pookie/version.go cmd/pookie/version_test.go cmd/pookie/main.go
git commit -m "feat(cli): add 'pookie version --check' for live release lookup"
```

---

## Task 6: Add background notifier to interactive commands

**Files:**
- Create: `cmd/pookie/notifier.go`
- Modify: `cmd/pookie/main.go` (call notifier helper at end of interactive paths)

The notifier must NOT fire for `run` (headless), `status` (machine-readable), `version --check` (already shows it), or any command in CI. Safe set to attach to: `start`, `chat`, `init`, the interactive menu, `list`, `doctor`.

- [ ] **Step 1: Write the failing test**

Append to `cmd/pookie/version_test.go`:

```go
func TestMaybeShowUpdateNoticeRespectsSkip(t *testing.T) {
	t.Setenv("CI", "true")
	var stderr bytes.Buffer
	maybeShowUpdateNotice(context.Background(), "0.5.2", &stderr, "https://invalid.invalid")
	if stderr.Len() != 0 {
		t.Fatalf("expected no output when CI=true, got %q", stderr.String())
	}
}

func TestMaybeShowUpdateNoticePrintsWhenNewer(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("POOKIEPAWS_NO_UPDATE_NOTIFIER", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://x"}`))
	}))
	defer srv.Close()

	// Override the cache path to a fresh tmp file so we always hit the network.
	t.Setenv("POOKIEPAWS_UPDATE_CACHE_PATH", filepath.Join(t.TempDir(), "uc.json"))

	var stderr bytes.Buffer
	maybeShowUpdateNotice(context.Background(), "0.5.2", &stderr, srv.URL)

	got := stderr.String()
	if !strings.Contains(got, "update available") {
		t.Fatalf("expected update notice, got %q", got)
	}
}

func TestMaybeShowUpdateNoticeSilentOnError(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("POOKIEPAWS_NO_UPDATE_NOTIFIER", "")
	t.Setenv("POOKIEPAWS_UPDATE_CACHE_PATH", filepath.Join(t.TempDir(), "uc.json"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	var stderr bytes.Buffer
	maybeShowUpdateNotice(context.Background(), "0.5.2", &stderr, srv.URL)
	if stderr.Len() != 0 {
		t.Fatalf("expected silent failure, got %q", stderr.String())
	}
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./cmd/pookie/... -run TestMaybeShowUpdateNotice -v`
Expected: FAIL — `maybeShowUpdateNotice` undefined.

- [ ] **Step 3: Implement `notifier.go`**

Create `cmd/pookie/notifier.go`:

```go
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mitpoai/pookiepaws/internal/updatecheck"
)

// maybeShowUpdateNotice runs the update check and, if a newer release is
// available, prints a single-line notice on stderr. It is fire-and-forget:
// any error (including network/timeout) is swallowed silently to avoid
// disrupting the user's actual command. The check is bounded to 1.5s.
//
// Set baseURL to "" for production use (defaults to the public GitHub API).
func maybeShowUpdateNotice(ctx context.Context, currentVersion string, stderr io.Writer, baseURL string) {
	if updatecheck.ShouldSkip() {
		return
	}
	cachePath := os.Getenv("POOKIEPAWS_UPDATE_CACHE_PATH")
	if cachePath == "" {
		cachePath = updatecheck.DefaultCachePath()
	}
	checkCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	notice, err := updatecheck.Check(checkCtx, updatecheck.Options{
		CurrentVersion: currentVersion,
		BaseURL:        baseURL,
		Cache:          updatecheck.NewCache(cachePath),
		Timeout:        1500 * time.Millisecond,
	})
	if err != nil || notice == nil {
		return
	}
	fmt.Fprintln(stderr, notice.FormatNotice())
}
```

- [ ] **Step 4: Wire into interactive commands in `cmd/pookie/main.go`**

Add a `defer` at the end of `cmdStart`, `cmdChat`, `cmdInit`, `cmdList`, and `cmdDoctor`. Locate each function (use `grep -n "^func cmdStart" cmd/pookie/*.go` and similar). Append immediately after the function's first line:

```go
	defer maybeShowUpdateNotice(context.Background(), version, os.Stderr, "")
```

For `launchInteractiveMenu`, add the same `defer` at the top of the function.

DO NOT add this to `cmdRun`, `cmdStatus`, `cmdSmoke`, `cmdAudit`, `cmdSessions`, `cmdApprovals`, `cmdContext`, `cmdMemory`, `cmdInstall` — these are non-interactive or machine-readable.

- [ ] **Step 5: Run tests**

Run: `go test ./cmd/pookie/... -v && go test ./internal/updatecheck/...`
Expected: PASS.

- [ ] **Step 6: Manual smoke — verify silent on error**

Run: `POOKIEPAWS_UPDATE_CACHE_PATH=/tmp/nonexistent-dir/uc.json go run ./cmd/pookie list`
Expected: `list` output unaffected; if no network, no error printed.

- [ ] **Step 7: Manual smoke — verify opt-out**

Run: `POOKIEPAWS_NO_UPDATE_NOTIFIER=1 go run ./cmd/pookie list`
Expected: no notice line on stderr regardless of release state.

- [ ] **Step 8: Commit**

```bash
git add cmd/pookie/notifier.go cmd/pookie/version_test.go cmd/pookie/main.go
git commit -m "feat(cli): show update notice on interactive runs (gh-style, opt-out via env)"
```

---

## Task 7: Document update behavior

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add README section**

In `README.md`, locate the "Installation" or "Getting Started" section. Add a new subsection at the same heading level:

```markdown
## Update Notifications

Pookie checks GitHub Releases for newer versions and prints a one-line notice
on stderr during interactive commands (`start`, `chat`, `init`, `list`,
`doctor`, and the menu). Results are cached for 24 hours under your OS user
cache directory.

To force a live check:

    pookie version --check

To opt out of the background notice:

    export POOKIEPAWS_NO_UPDATE_NOTIFIER=1

The notifier is also automatically disabled when `CI` is set in the
environment. Pookie never installs updates for you — the notice points to
`winget`, `brew`, or the install script depending on your platform.
```

- [ ] **Step 2: Add CHANGELOG entry**

In `CHANGELOG.md`, add under the unreleased section (or create one):

```markdown
## [Unreleased]

### Added
- `pookie version --check` performs a live lookup against GitHub Releases.
- Background update notifier on interactive commands; cached 24h, opt-out
  via `POOKIEPAWS_NO_UPDATE_NOTIFIER=1` or any `CI=*` value.
- `internal/updatecheck` package (release fetch, semver compare, file cache).
```

- [ ] **Step 3: Final test pass**

Run: `go test ./...`
Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add README.md CHANGELOG.md
git commit -m "docs: describe update notifier and POOKIEPAWS_NO_UPDATE_NOTIFIER opt-out"
```

---

## Verification Summary

After all tasks:

- `go test ./...` — green
- `go build ./...` — clean
- `pookie version` — prints version line, no network
- `pookie version --check` — prints `latest:` or `up to date`, hits network
- `pookie list` (interactive) — works as before, plus optional one-line notice on stderr if outdated
- `CI=true pookie list` — no notice
- `POOKIEPAWS_NO_UPDATE_NOTIFIER=1 pookie list` — no notice
- Cache file exists at `os.UserCacheDir()/pookiepaws/update-check.json` after a check

## Out of scope (deferred)

- Binary self-updater (`pookie self-upgrade`) — explicitly not built.
- WinGet/Homebrew packaging itself — covered in Plan 4.
- `pookie research` and scheduler — covered in Plan 2.
- UI watchlist migration — covered in Plan 3.
