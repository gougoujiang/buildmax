package k8s

import (
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/util"
)

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
			if got := workerJobNameForTaskRunAt(tt.taskRunID, now); got != tt.want {
				t.Fatalf("WorkerJobNameForTaskRunAt(%q) = %q, want %q", tt.taskRunID, got, tt.want)
			}
		})
	}
}

func TestWorkerJobNameForTaskRunAt_TruncatesBase(t *testing.T) {
	got := workerJobNameForTaskRunAt("r_"+strings.Repeat("abc", 20), time.Unix(1700000000, 0))
	wantPrefix := "buildmax-worker-r-abcabcabcabcabcabcabcabcabca-"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("WorkerJobNameForTaskRunAt() prefix = %q, want prefix %q", got, wantPrefix)
	}
	if len(got) > 63 {
		t.Fatalf("WorkerJobNameForTaskRunAt() length = %d, want <= 63", len(got))
	}
}

// TestWorkerJobNameForTaskRunAt_PublicIDSurvivesSanitizing is the reason public
// IDs are base32 rather than base64url. This sanitizer lowercases its input and
// rewrites every character Kubernetes will not take, and its only suffix is a
// second-resolution timestamp — so an encoding that folds under those rules can
// give two runs created in the same second one Job name.
func TestWorkerJobNameForTaskRunAt_PublicIDSurvivesSanitizing(t *testing.T) {
	id, err := util.NewPublicID()
	if err != nil {
		t.Fatalf("NewPublicID() error = %v", err)
	}
	got := workerJobNameForTaskRunAt(id, time.Unix(1700000000, 0))
	want := "buildmax-worker-" + id + "-1700000000"
	if got != want {
		t.Fatalf("WorkerJobNameForTaskRunAt(%q) = %q, want %q", id, got, want)
	}
	if len(got) > 63 {
		t.Fatalf("WorkerJobNameForTaskRunAt(%q) length = %d, want <= 63", id, len(got))
	}
}
