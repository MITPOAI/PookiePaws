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
	// POOKIEPAWS_UPDATE_CACHE_PATH lets tests redirect the update-check
	// cache to a temp file. It is also a documented escape hatch for users
	// who want to pin the cache to a non-default location (see README).
	cachePath := os.Getenv("POOKIEPAWS_UPDATE_CACHE_PATH")
	if cachePath == "" {
		cachePath = updatecheck.DefaultCachePath()
	}
	checkCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	// Both ctx-deadline and Options.Timeout are required: ctx covers
	// cancellation propagation; Options.Timeout configures the underlying
	// HTTP client's transport-level timeout.
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
