package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New initializes a new slog.Logger
// Log level can be debug, info, warn, error
func New(level string) *slog.Logger {
	var logLevel slog.Level

	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo // Default to info
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
		// AddSource: true, // Uncomment to include source file and line number
	}

	// Using JSONHandler for structured logging, can be TextHandler for development
	handler := slog.NewJSONHandler(os.Stdout, opts)
	// handler := slog.NewTextHandler(os.Stdout, opts) // Alternative for local dev

	logger := slog.New(handler)
	return logger
}
