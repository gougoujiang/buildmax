// Package log configures the application's default slog logger. Init accepts
// explicit logsDir and level so the package has no dependency on config.
// When alsoStdout is true logs go to both file and stdout (for server/worker);
// when false, file only (for CLI so TUI and prompt-mode stdout stay clean).
// SetOutput is provided for tests.
package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	// Lumberjack defaults for the persisted log file.
	logMaxSizeMB  = 10
	logMaxBackups = 3
	logMaxAgeDays = 7
	logCompress   = true
	logFilename   = "buildmax.log"
)

// currentLevel is the minimum level set in Init(); used by SetOutput to build
// a handler with the same level. Defaults to Info so SetOutput without Init still works.
var currentLevel = slog.LevelInfo

// fileWriter is the rotating log file writer, if Init() created one. Used by DisableConsole.
var fileWriter io.Writer

// LogConfig holds configuration for log initialization.
type LogConfig struct {
	LogsDir    string
	Level      string
	Filename   string
	AlsoStdout bool
}

// Init configures slog.Default() with the given config.
// It creates LogsDir if needed and sets output to a rotating file there.
// If Filename is empty, "buildmax.log" is used. When AlsoStdout is true,
// logs go to both the file and os.Stdout; when false, file only.
func Init(cfg LogConfig) {
	parsedLevel := parseLevel(cfg.Level)
	currentLevel = parsedLevel

	chosenName := cfg.Filename
	if chosenName == "" {
		chosenName = logFilename
	}

	if err := os.MkdirAll(cfg.LogsDir, 0750); err != nil {
		fileWriter = nil
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: parsedLevel})))
		return
	}

	lj := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogsDir, chosenName),
		MaxSize:    logMaxSizeMB,
		MaxBackups: logMaxBackups,
		MaxAge:     logMaxAgeDays,
		Compress:   logCompress,
	}
	fileWriter = lj
	out := io.Writer(lj)
	if cfg.AlsoStdout {
		out = io.MultiWriter(lj, os.Stdout)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: parsedLevel})))
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
