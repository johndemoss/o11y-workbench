package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

var logger *slog.Logger

func init() {
	level := slog.LevelInfo // default to INFO

	if logLevel := os.Getenv("O11Y_WORKBENCH_LOG_LEVEL"); logLevel != "" {
		level = parseLevelString(logLevel)
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	logger = slog.New(handler)
}

func parseLevelString(s string) slog.Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		slog.Warn("Invalid log level, defaulting to INFO", "requested", s)
		return slog.LevelInfo
	}
}

// SetLogger allows replacing the default logger (useful for OTel bridge integration)
func SetLogger(l *slog.Logger) {
	logger = l
}

// GetLogger returns the current logger instance
func GetLogger() *slog.Logger {
	return logger
}

func Debugf(format string, args ...any) {
	logger.Debug(fmt.Sprintf(format, args...))
}

func Infof(format string, args ...any) {
	logger.Info(fmt.Sprintf(format, args...))
}

func Warnf(format string, args ...any) {
	logger.Warn(fmt.Sprintf(format, args...))
}

func Errorf(format string, args ...any) {
	logger.Error(fmt.Sprintf(format, args...))
}
