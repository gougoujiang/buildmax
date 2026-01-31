// Package log configures the application's default slog logger: level from
// BUILDMAX_LOG_LEVEL, dual output (stderr + rotating file under config.DataDir()/logs),
// and SetOutput for tests.
package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"buildmax/internal/config"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	// Lumberjack defaults for the persisted log file.
	logMaxSizeMB   = 10
	logMaxBackups  = 3
	logMaxAgeDays  = 7
	logCompress    = true
	logFilename    = "buildmax.log"
	logsSubdir     = "logs"
)

// currentLevel is the minimum level set in Init(); used by SetOutput to build
// a handler with the same level. Defaults to Info so SetOutput without Init still works.
var currentLevel = slog.LevelInfo

// Init configures slog.Default() with level from BUILDMAX_LOG_LEVEL, creates
// config.DataDir()/logs, and sets output to both stderr and a rotating file
// (buildmax.log) via Lumberjack.
func Init() {
	level := parseLevel(os.Getenv("BUILDMAX_LOG_LEVEL"))
	currentLevel = level

	logsDir := filepath.Join(config.DataDir(), logsSubdir)
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		// Fall back to stderr only if we cannot create logs dir
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
		return
	}

	lj := &lumberjack.Logger{
		Filename:   filepath.Join(logsDir, logFilename),
		MaxSize:    logMaxSizeMB,
		MaxBackups: logMaxBackups,
		MaxAge:     logMaxAgeDays,
		Compress:   logCompress,
	}

	w := io.MultiWriter(os.Stderr, lj)
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
}

// SetOutput replaces slog.Default() with a logger that writes only to w,
// using the current minimum level (from Init or default Info). Used by tests
// to capture log output without writing to stderr or the real file.
func SetOutput(w io.Writer) {
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: currentLevel})))
}

// parseLevel maps s (case-insensitive) to a slog.Level: "debug", "info", "warn",
// "error", "off". Invalid or empty returns slog.LevelInfo. "off" returns a level
// above Error so no messages are written.
func parseLevel(s string) slog.Level {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "off":
		return slog.LevelError + 1
	default:
		return slog.LevelInfo
	}
}
