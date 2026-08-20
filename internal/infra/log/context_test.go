package log

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func capture(t *testing.T) *bytes.Buffer {
	t.Helper()
	Init(LogConfig{LogsDir: t.TempDir(), Level: "debug"})
	var buf bytes.Buffer
	SetOutput(&buf)
	return &buf
}

func TestContextAttrsReachTheRecord(t *testing.T) {
	buf := capture(t)
	ctx := With(context.Background(), "request_id", "rq_1")

	slog.InfoContext(ctx, "handled")

	if out := buf.String(); !strings.Contains(out, "request_id=rq_1") {
		t.Errorf("context attr missing from record: %q", out)
	}
}

func TestContextAttrsAccumulate(t *testing.T) {
	buf := capture(t)
	ctx := With(With(context.Background(), "request_id", "rq_1"), "task_run_id", "r_1")

	slog.InfoContext(ctx, "handled")

	out := buf.String()
	for _, want := range []string{"request_id=rq_1", "task_run_id=r_1"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in %q", want, out)
		}
	}
}

// With must not write through to a context another goroutine already holds.
func TestWithDoesNotMutateParent(t *testing.T) {
	buf := capture(t)
	parent := With(context.Background(), "request_id", "rq_1")
	_ = With(parent, "user_id", "u_1")

	slog.InfoContext(parent, "handled")

	if out := buf.String(); strings.Contains(out, "user_id") {
		t.Errorf("child attr leaked into parent context: %q", out)
	}
}

// slog.With returns a logger built through Handler.WithAttrs. If that path drops
// the wrapper, every logger derived from the default one stops reading context.
func TestDerivedLoggerStillReadsContext(t *testing.T) {
	buf := capture(t)
	ctx := With(context.Background(), "request_id", "rq_1")

	slog.With("component", "scheduler").InfoContext(ctx, "claimed")

	out := buf.String()
	for _, want := range []string{"component=scheduler", "request_id=rq_1"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in %q", want, out)
		}
	}
}

func TestGroupedLoggerStillReadsContext(t *testing.T) {
	buf := capture(t)
	ctx := With(context.Background(), "request_id", "rq_1")

	slog.Default().WithGroup("http").InfoContext(ctx, "handled")

	if out := buf.String(); !strings.Contains(out, "request_id=rq_1") {
		t.Errorf("context attr lost through WithGroup: %q", out)
	}
}

func TestNoContextAttrsIsUnchanged(t *testing.T) {
	buf := capture(t)

	slog.InfoContext(context.Background(), "plain")
	slog.Info("also plain")

	if out := buf.String(); strings.Count(out, "plain") != 2 {
		t.Errorf("want both records, got %q", out)
	}
}
