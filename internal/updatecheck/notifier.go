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
		entry, err := opts.Cache.Load()
		if err != nil {
			return nil, fmt.Errorf("load cache: %w", err)
		}
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
		if err := opts.Cache.Save(&CacheEntry{
			CheckedAt: time.Now().UTC(),
			Release:   *fetched,
		}); err != nil {
			return nil, fmt.Errorf("save cache: %w", err)
		}
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

// CheckCacheOnly returns a Notice if the on-disk cache is fresh and reports a
// newer release. It never makes a network call: missing, expired, or corrupt
// caches return (nil, nil). Errors are limited to genuine I/O problems while
// reading the cache file. Useful for surfacing the cached upgrade hint on
// `pookie version` without paying any network latency.
func CheckCacheOnly(opts Options) (*Notice, error) {
	if opts.TTL == 0 {
		opts.TTL = 24 * time.Hour
	}
	if opts.Cache == nil {
		return nil, fmt.Errorf("updatecheck.CheckCacheOnly: Cache is required")
	}
	entry, err := opts.Cache.Load()
	if err != nil {
		return nil, fmt.Errorf("load cache: %w", err)
	}
	if entry == nil {
		return nil, nil
	}
	if entry.IsExpired(opts.TTL, time.Now().UTC()) {
		return nil, nil
	}
	if !IsNewer(opts.CurrentVersion, entry.Release.TagName) {
		return nil, nil
	}
	return &Notice{
		Current: opts.CurrentVersion,
		Latest:  entry.Release.TagName,
		URL:     entry.Release.HTMLURL,
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

// FormatNotice renders a two-line stderr message: the headline on the first
// line and the upgrade hint on the second.
func (n *Notice) FormatNotice() string {
	return fmt.Sprintf("update available: %s → %s  (%s)\n  upgrade: %s",
		n.Current, n.Latest, n.URL, n.Hint)
}
