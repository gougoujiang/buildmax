package k8s

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Kubernetes names an object with a DNS-1123 label: lower-case alphanumerics
// and dashes, at most 63 characters. A task run id is neither, so it is
// sanitized rather than rejected.
var (
	nonDNS1123Chars = regexp.MustCompile(`[^a-z0-9-]+`)
	repeatedDashes  = regexp.MustCompile(`-+`)
)

// workerJobNameForTaskRun returns a DNS-1123-compatible worker Job name for a
// task run id.
func workerJobNameForTaskRun(taskRunID string) string {
	return workerJobNameForTaskRunAt(taskRunID, time.Now())
}

// workerJobNameForTaskRunAt returns a DNS-1123-compatible worker Job name for a
// task run id and timestamp. Total length is kept <= 63 characters.
//
// The timestamp is part of the name because a retry launches a second Job for
// the same run, and two Jobs cannot share a name.
func workerJobNameForTaskRunAt(taskRunID string, now time.Time) string {
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
