package audit_test

import (
	"context"
	"testing"

	audit "github.com/icloudbb/buildmax/internal/core/audit"
)

func TestRunContextRoundTrips(t *testing.T) {
	ctx := audit.ContextWithRun(context.Background(), "r_123")
	if got := audit.RunFromContext(ctx); got != "r_123" {
		t.Errorf("RunFromContext = %q, want r_123", got)
	}
}

// An empty id leaves the context untagged rather than storing a blank, so a
// downstream write does not read one run's absence as another's presence.
func TestEmptyRunIsNotTagged(t *testing.T) {
	if got := audit.RunFromContext(audit.ContextWithRun(context.Background(), "")); got != "" {
		t.Errorf("RunFromContext = %q, want empty", got)
	}
	if got := audit.RunFromContext(context.Background()); got != "" {
		t.Errorf("RunFromContext on a bare context = %q, want empty", got)
	}
}
