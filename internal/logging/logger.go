package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Logger is a thin wrapper that keeps the logging dependency narrow.
type Logger struct {
	inner *slog.Logger
}

// NewLogger creates a text logger with an explicit log level.
func NewLogger(level string) (*Logger, error) {
	parsedLevel, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: parsedLevel,
	})

	return &Logger{
		inner: slog.New(handler),
	}, nil
}

// Slog exposes the underlying logger for packages that already depend on slog.
func (l *Logger) Slog() *slog.Logger {
	if l == nil {
		return slog.Default()
	}
	return l.inner
}

// Info forwards structured info logs to the underlying slog.Logger.
func (l *Logger) Info(message string, args ...any) {
	l.Slog().Info(message, args...)
}

// Warn forwards structured warning logs to the underlying slog.Logger.
func (l *Logger) Warn(message string, args ...any) {
	l.Slog().Warn(message, args...)
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", value)
	}
}
