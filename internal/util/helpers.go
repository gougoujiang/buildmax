package util

import (
	"fmt"
	"os"
	"time"
	"unicode/utf8"
)

// WithEnvVar sets envKey to value for the duration of fn, then restores the previous process env state.
func WithEnvVar(envKey, value string, fn func() error) error {
	prev, hadPrev := os.LookupEnv(envKey)
	if err := os.Setenv(envKey, value); err != nil {
		return err
	}
	defer func() {
		if hadPrev {
			_ = os.Setenv(envKey, prev)
		} else {
			_ = os.Unsetenv(envKey)
		}
	}()
	return fn()
}

// Ptr returns a pointer to v. Useful for filling optional pointer fields.
func Ptr[T any](v T) *T {
	return &v
}

// FormatDuration formats a duration in a compact human-readable way.
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}

// FormatMinute formats an instant as YYYY-MM-DD HH:MM in local time.
//
// The reader is a person — a tool result or a listing — so local time is the
// useful rendering. Stored and transported instants stay UTC.
func FormatMinute(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04")
}

// TruncateRunes truncates s to at most maxRunes runes and appends an ellipsis
// when truncation happens. If maxRunes is non-positive, it returns the empty string.
func TruncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}

// ClipRunes returns at most the first maxRunes runes of s without adding a suffix.
// If maxRunes is non-positive, it returns the empty string.
func ClipRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}
