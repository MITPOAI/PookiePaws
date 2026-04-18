package main

import (
	"log/slog"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"warning", slog.LevelWarn},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"err", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}
	for _, tc := range cases {
		got := parseLogLevel(tc.in)
		if got != tc.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSchedulerLoggerAdapter(t *testing.T) {
	logger := slog.Default()
	adapter := schedulerLoggerAdapter(logger)
	// Smoke test: invoke each level — should not panic
	adapter("debug", "test", "k", "v")
	adapter("info", "test")
	adapter("warn", "test")
	adapter("error", "test")
	adapter("unknown", "test")
}
