package main

import (
	"log/slog"
	"os"
	"strings"
)

// initLogger configures the default slog logger from env vars and returns it.
// Called once from main(). Subsequent slog.Info/Warn/Error calls route here.
//
// POOKIEPAWS_LOG_FORMAT: text (default) | json
// POOKIEPAWS_LOG_LEVEL:  debug | info (default) | warn | error
func initLogger() *slog.Logger {
	format := strings.ToLower(strings.TrimSpace(os.Getenv("POOKIEPAWS_LOG_FORMAT")))
	level := parseLogLevel(os.Getenv("POOKIEPAWS_LOG_LEVEL"))

	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	switch format {
	case "json":
		h = slog.NewJSONHandler(os.Stderr, opts)
	default:
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	logger := slog.New(h)
	slog.SetDefault(logger)
	return logger
}

// parseLogLevel maps a string to an slog.Level. Unknown values return
// slog.LevelInfo so misconfiguration never silences logs.
func parseLogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// schedulerLoggerAdapter bridges scheduler.Logger (a func) to an slog.Logger.
// Levels other than debug/warn/error map to Info.
func schedulerLoggerAdapter(logger *slog.Logger) func(level, msg string, kvs ...any) {
	return func(level, msg string, kvs ...any) {
		switch strings.ToLower(level) {
		case "debug":
			logger.Debug(msg, kvs...)
		case "warn", "warning":
			logger.Warn(msg, kvs...)
		case "error":
			logger.Error(msg, kvs...)
		default:
			logger.Info(msg, kvs...)
		}
	}
}
