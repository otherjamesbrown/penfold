package main

import (
	"log/slog"
	"os"
	"strings"
)

// initLogging configures the global slog logger with a JSON handler.
// Log level is read from PENFOLD_LOG_LEVEL env var (default: info).
func initLogging() {
	level := parseLogLevel(os.Getenv("PENFOLD_LOG_LEVEL"))
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
