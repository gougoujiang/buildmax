package log

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"buildmax/internal/config"
)

func TestLevelFiltering(t *testing.T) {
	// Use temp dir so Init() does not touch real ~/.buildmax
	tmp := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, tmp)
	t.Setenv(config.EnvKeyBuildmaxLogLevel, "info")
	Init()

	var buf bytes.Buffer
	SetOutput(&buf)

	slog.Debug("x")
	if buf.Len() > 0 && strings.Contains(buf.String(), "x") {
		t.Error("Debug message should not be written when level is Info")
	}

	buf.Reset()
	slog.Info("y")
	out := buf.String()
	if !strings.Contains(out, "y") {
		t.Error("Info message should be written when level is Info")
	}
	if !strings.Contains(out, "INFO") {
		t.Error("output should contain level INFO")
	}
}

func TestEnvDefault_Debug(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, tmp)
	t.Setenv(config.EnvKeyBuildmaxLogLevel, "debug")
	Init()

	var buf bytes.Buffer
	SetOutput(&buf)

	slog.Debug("z")
	out := buf.String()
	if !strings.Contains(out, "z") {
		t.Error("Debug message should be written when BUILDMAX_LOG_LEVEL=debug")
	}
	if !strings.Contains(out, "DEBUG") {
		t.Error("output should contain level DEBUG")
	}
}

func TestOutputContent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, tmp)
	t.Setenv(config.EnvKeyBuildmaxLogLevel, "info")
	Init()

	var buf bytes.Buffer
	SetOutput(&buf)

	msg := "test message content"
	slog.Info(msg)
	out := buf.String()
	if !strings.Contains(out, msg) {
		t.Errorf("output should contain message %q, got %q", msg, out)
	}
	if !strings.Contains(out, "INFO") {
		t.Error("output should contain level INFO")
	}
}

func TestParseLevel_InvalidDefaultsToInfo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, tmp)
	t.Setenv(config.EnvKeyBuildmaxLogLevel, "invalid")
	Init()

	var buf bytes.Buffer
	SetOutput(&buf)

	slog.Info("default-level")
	if !strings.Contains(buf.String(), "default-level") {
		t.Error("invalid BUILDMAX_LOG_LEVEL should default to Info; Info should be written")
	}
	buf.Reset()
	slog.Debug("debug-should-not-appear")
	if strings.Contains(buf.String(), "debug-should-not-appear") {
		t.Error("invalid BUILDMAX_LOG_LEVEL defaults to Info; Debug should not be written")
	}
}
