package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/util"
)

func TestSlashCompactIsOffered(t *testing.T) {
	if !slices.Contains(builtinSlashCommands, "/compact") {
		t.Fatalf("/compact missing from the completion list: %v", builtinSlashCommands)
	}
}

// TestSlashCompactWithoutASessionExplainsItself pins that the command answers rather than
// starting work it cannot do: without a session there is no history to summarize.
func TestSlashCompactWithoutASessionExplainsItself(t *testing.T) {
	m := NewModel(TUIOpts{Workspace: util.FixedRoot(t.TempDir())})
	next, _ := dispatchSlashCommand(m, "/compact")
	mod := next.(*Model)
	if mod.busy {
		t.Error("/compact went busy with no session to compact")
	}
	if mod.err == "" {
		t.Error("/compact with no session should say why nothing happened")
	}
}

// TestBusyHintNamesCompaction covers the reason busyLabel exists: a compaction is not a turn,
// and "Generating" would describe a reply that is not coming.
func TestBusyHintNamesCompaction(t *testing.T) {
	m := NewModel(TUIOpts{Session: testSessionContext(), Workspace: util.FixedRoot(t.TempDir())})
	m.busy = true
	m.busyLabel = "Compacting"
	if got := m.View().Content; !strings.Contains(got, "Compacting") {
		t.Errorf("busy hint = %q, want it to name the compaction", got)
	}
}

func TestRenderCompacted(t *testing.T) {
	got := renderCompacted(agentapp.CompactResult{
		Summarized:   12,
		Kept:         3,
		BeforeTokens: 90_000,
		Status:       agentapp.RunUsage{ContextTokens: 9_000},
	})
	for _, want := range []string{"12 messages", "3 messages", "9,000 tokens", "90,000"} {
		if !strings.Contains(got, want) {
			t.Errorf("report = %q, want it to contain %q", got, want)
		}
	}

	skipped := renderCompacted(agentapp.CompactResult{Reason: "blocked by a PreCompact hook"})
	if !strings.Contains(skipped, "blocked by a PreCompact hook") {
		t.Errorf("report = %q, want the reason nothing was compacted", skipped)
	}
}
