// Package log configures the application's default slog logger: level from
// BUILDMAX_LOG_LEVEL, rotating file under config.DataDir()/logs. Init accepts a
// filename (empty = "buildmax.log") and alsoStdout; when alsoStdout is true logs
// go to both file and stdout (for server/worker); when false, file only (for CLI
// so TUI and prompt-mode stdout stay clean). SetOutput is provided for tests.
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
)

// currentLevel is the minimum level set in Init(); used by SetOutput to build
// a handler with the same level. Defaults to Info so SetOutput without Init still works.
var currentLevel = slog.LevelInfo

// fileWriter is the rotating log file writer, if Init() created one. Used by DisableConsole.
var fileWriter io.Writer

// Init configures slog.Default() with level from config.LogLevel() (BUILDMAX_LOG_LEVEL), creates
// config.DataDir()/logs, and sets output to a rotating file. If filename is empty, "buildmax.log" is used.
// When alsoStdout is true, logs go to both the file and os.Stdout; when false, file only.
func Init(filename string, alsoStdout bool) {
	level := parseLevel(config.LogLevel())
	currentLevel = level

	chosenName := filename
	if chosenName == "" {
		chosenName = logFilename
	}

	logsDir := config.LogsDir()
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		fileWriter = nil
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: level})))
		return
	}

	lj := &lumberjack.Logger{
		Filename:   filepath.Join(logsDir, chosenName),
		MaxSize:    logMaxSizeMB,
		MaxBackups: logMaxBackups,
		MaxAge:     logMaxAgeDays,
		Compress:   logCompress,
	}
	fileWriter = lj
	out := io.Writer(lj)
	if alsoStdout {
		out = io.MultiWriter(lj, os.Stdout)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})))
}

// DisableConsole reconfigures slog.Default() to write only to the file (or
// discard if no file). Init() already uses file-only output; this is for
// callers that need to re-apply file-only after changing the default logger.
func DisableConsole() {
	if fileWriter != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(fileWriter, &slog.HandlerOptions{Level: currentLevel})))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: currentLevel})))
	}
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
