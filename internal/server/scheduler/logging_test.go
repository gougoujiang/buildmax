package scheduler

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	buildmaxlog "github.com/gougoujiang/buildmax/internal/infra/log"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buildmaxlog.Init(buildmaxlog.LogConfig{LogsDir: t.TempDir(), Level: "debug"})
	var buf bytes.Buffer
	buildmaxlog.SetOutput(&buf)
	return &buf
}

// Each loop in this package used to spell its own identity into the message,
// three different ways, and one of them not at all. A component attr is what
// makes "show me the scheduler" a filter rather than a guess.
func TestEachLoopTagsItsComponent(t *testing.T) {
	for _, tc := range []struct {
		logger *slog.Logger
		want   string
	}{
		{(&Scheduler{}).log(), "component=scheduler"},
		{(&CredentialCleaner{}).log(), "component=credential_cleaner"},
		{(&StaleRunReaper{}).log(), "component=stale_run_reaper"},
		{(&AuditRetainer{}).log(), "component=audit_retention"},
		{componentLog("worker_runner"), "component=worker_runner"},
	} {
		buf := captureLog(t)
		tc.logger.Info("started")
		if out := buf.String(); !strings.Contains(out, tc.want) {
			t.Errorf("want %q in %q", tc.want, out)
		}
	}
}

// The component logger is derived from slog.Default(), so it has to be built
// after infra/log installs the real handler. A package-level var would capture
// the startup default and write somewhere nobody is reading.
func TestComponentLoggerFollowsTheCurrentDefault(t *testing.T) {
	buildmaxlog.Init(buildmaxlog.LogConfig{LogsDir: t.TempDir(), Level: "debug"})
	var first, second bytes.Buffer

	buildmaxlog.SetOutput(&first)
	componentLog("scheduler").Info("one")
	buildmaxlog.SetOutput(&second)
	componentLog("scheduler").Info("two")

	if !strings.Contains(first.String(), "one") || strings.Contains(first.String(), "two") {
		t.Errorf("first buffer should hold only the first record: %q", first.String())
	}
	if !strings.Contains(second.String(), "two") {
		t.Errorf("second buffer should hold the second record: %q", second.String())
	}
}

// A run id set once on the context has to reach records that never mention it.
func TestRunIDOnContextReachesRecords(t *testing.T) {
	buf := captureLog(t)
	ctx := buildmaxlog.With(context.Background(), "task_run_id", "r_1")

	(&Scheduler{}).log().ErrorContext(ctx, "worker spawn failed; marking run FAILED", "err", "boom")

	out := buf.String()
	for _, want := range []string{"component=scheduler", "task_run_id=r_1", "level=ERROR"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in %q", want, out)
		}
	}
}
