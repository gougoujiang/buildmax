package util

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestWithEnvVar_RestoresExistingValue(t *testing.T) {
	const key = "BUILDMAX_TEST_ENV_RESTORE_EXISTING"
	if err := os.Setenv(key, "before"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv(key) }()

	if err := WithEnvVar(key, "during", func() error {
		got := os.Getenv(key)
		if got != "during" {
			t.Fatalf("inside WithEnvVar: got %q, want during", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithEnvVar: %v", err)
	}

	if got := os.Getenv(key); got != "before" {
		t.Fatalf("after WithEnvVar: got %q, want before", got)
	}
}

func TestWithEnvVar_RestoresUnsetState(t *testing.T) {
	const key = "BUILDMAX_TEST_ENV_RESTORE_UNSET"
	_ = os.Unsetenv(key)

	if err := WithEnvVar(key, "during", func() error {
		got := os.Getenv(key)
		if got != "during" {
			t.Fatalf("inside WithEnvVar: got %q, want during", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithEnvVar: %v", err)
	}

	if _, ok := os.LookupEnv(key); ok {
		t.Fatalf("after WithEnvVar: %s should be unset", key)
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxRunes int
		want     string
	}{
		{name: "non_positive_limit", input: "hello", maxRunes: 0, want: ""},
		{name: "shorter_than_limit", input: "hello", maxRunes: 10, want: "hello"},
		{name: "equal_to_limit", input: "hello", maxRunes: 5, want: "hello"},
		{name: "truncate_ascii", input: "hello world", maxRunes: 5, want: "hello…"},
		{name: "truncate_unicode", input: "你好世界再见", maxRunes: 4, want: "你好世界…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncateRunes(tt.input, tt.maxRunes); got != tt.want {
				t.Fatalf("TruncateRunes(%q, %d) = %q, want %q", tt.input, tt.maxRunes, got, tt.want)
			}
		})
	}
}

func TestPtr(t *testing.T) {
	s := Ptr("hello")
	if s == nil || *s != "hello" {
		t.Fatalf("Ptr(string) = %v, want pointer to hello", s)
	}

	n := Ptr(int32(3))
	if n == nil || *n != 3 {
		t.Fatalf("Ptr(int32) = %v, want pointer to 3", n)
	}
}

func TestClipRunes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxRunes int
		want     string
	}{
		{name: "non_positive_limit", input: "hello", maxRunes: 0, want: ""},
		{name: "shorter_than_limit", input: "hello", maxRunes: 10, want: "hello"},
		{name: "equal_to_limit", input: "hello", maxRunes: 5, want: "hello"},
		{name: "clip_ascii", input: "hello world", maxRunes: 5, want: "hello"},
		{name: "clip_unicode", input: "你好世界再见", maxRunes: 4, want: "你好世界"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClipRunes(tt.input, tt.maxRunes); got != tt.want {
				t.Fatalf("ClipRunes(%q, %d) = %q, want %q", tt.input, tt.maxRunes, got, tt.want)
			}
		})
	}
}

func TestWorkerJobNameForTaskRunAt(t *testing.T) {
	now := time.Unix(1700000000, 0)

	tests := []struct {
		name      string
		taskRunID string
		want      string
	}{
		{
			name:      "preserves_basic_id",
			taskRunID: "r_abc-123",
			want:      "buildmax-worker-r-abc-123-1700000000",
		},
		{
			name:      "normalizes_invalid_chars",
			taskRunID: "R__ABC/123",
			want:      "buildmax-worker-r-abc-123-1700000000",
		},
		{
			name:      "falls_back_when_empty_after_sanitize",
			taskRunID: "___",
			want:      "buildmax-worker-task-1700000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorkerJobNameForTaskRunAt(tt.taskRunID, now); got != tt.want {
				t.Fatalf("WorkerJobNameForTaskRunAt(%q) = %q, want %q", tt.taskRunID, got, tt.want)
			}
		})
	}
}

func TestWorkerJobNameForTaskRunAt_TruncatesBase(t *testing.T) {
	got := WorkerJobNameForTaskRunAt("r_"+strings.Repeat("abc", 20), time.Unix(1700000000, 0))
	wantPrefix := "buildmax-worker-r-abcabcabcabcabcabcabcabcabca-"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("WorkerJobNameForTaskRunAt() prefix = %q, want prefix %q", got, wantPrefix)
	}
	if len(got) > 63 {
		t.Fatalf("WorkerJobNameForTaskRunAt() length = %d, want <= 63", len(got))
	}
}

func TestFormatUnixMinute(t *testing.T) {
	got := FormatUnixMinute(1700000000)
	if got != "2023-11-15 06:13" {
		t.Fatalf("FormatUnixMinute() = %q, want %q", got, "2023-11-15 06:13")
	}
}
