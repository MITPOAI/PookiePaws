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
		// Best-effort cache-only hint: print the cached upgrade notice if one
		// exists, but never make a network call and never surface errors. The
		// user asked for version info, not for a notifier-style nudge — so we
		// stay silent when the cache is empty, expired, missing, or unreadable.
		if cfg.CachePath == "" {
			return nil
		}
		notice, err := updatecheck.CheckCacheOnly(updatecheck.Options{
			CurrentVersion: cfg.Version,
			Cache:          updatecheck.NewCache(cfg.CachePath),
		})
		if err == nil && notice != nil {
			fmt.Fprintf(cfg.Stdout, "latest:  %s\n  url:   %s\n  upgrade: %s\n",
				notice.Latest, notice.URL, notice.Hint)
		}
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
