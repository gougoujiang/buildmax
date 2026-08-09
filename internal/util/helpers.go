package util

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	nonDNS1123Chars = regexp.MustCompile(`[^a-z0-9-]+`)
	repeatedDashes  = regexp.MustCompile(`-+`)
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

// FormatUnixMinute formats a unix timestamp as YYYY-MM-DD HH:MM.
func FormatUnixMinute(unixSec int64) string {
	return time.Unix(unixSec, 0).Format("2006-01-02 15:04")
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

// WorkerJobNameForTaskRun returns a DNS-1123-compatible worker Job name for a task run id.
func WorkerJobNameForTaskRun(taskRunID string) string {
	return WorkerJobNameForTaskRunAt(taskRunID, time.Now())
}

// WorkerJobNameForTaskRunAt returns a DNS-1123-compatible worker Job name for a task run id and timestamp.
// Total length is kept <= 63 characters.
func WorkerJobNameForTaskRunAt(taskRunID string, now time.Time) string {
	const maxBaseLen = 30

	sanitized := strings.ToLower(taskRunID)
	sanitized = nonDNS1123Chars.ReplaceAllString(sanitized, "-")
	sanitized = repeatedDashes.ReplaceAllString(sanitized, "-")
	sanitized = strings.Trim(sanitized, "-")
	if len(sanitized) > maxBaseLen {
		sanitized = sanitized[:maxBaseLen]
	}
	if sanitized == "" {
		sanitized = "task"
	}

	ts := strconv.FormatInt(now.Unix(), 10)
	return "buildmax-worker-" + sanitized + "-" + ts
}
