package agentapp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/infra/sessionstore"
)

func TestForkCopiesTheHistoryThroughTheChosenMessage(t *testing.T) {
	m, parent := openManaged(t)
	seedTurns(t, parent)

	points := RewindPoints(parent)
	through := points[1] // the assistant "ok"

	child, err := m.Fork(parent, through.ItemID, "test-model")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	defer func() { _ = child.Close() }()

	msgs := child.Messages()
	if len(msgs) != 2 || msgs[0].Content != "first" || msgs[1].Content != "ok" {
		t.Fatalf("child messages = %#v, want the prefix through the chosen message", msgs)
	}
	if child.ID() == parent.ID() {
		t.Error("the child reused the parent's id")
	}
	from := child.Meta().ForkedFrom
	if from == nil || from.SessionID != parent.ID() || from.HeadID != through.ItemID {
		t.Errorf("forked_from = %+v, want the parent and the chosen head", from)
	}
	// The parent is untouched and still usable: forking is not leaving.
	if len(parent.Messages()) != 5 {
		t.Errorf("parent messages = %d, want its own history intact", len(parent.Messages()))
	}
	if err := parent.Append(llm.Message{Role: "user", Content: "parent carries on"}); err != nil {
		t.Errorf("the parent could not continue after being forked: %v", err)
	}
}

func TestForkedChildSurvivesTheParentBeingDeleted(t *testing.T) {
	m, parent := openManaged(t)
	seedTurns(t, parent)
	parentID := parent.ID()

	child, err := m.Fork(parent, RewindPoints(parent)[1].ItemID, "test-model")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	childID := child.ID()
	if err := child.Close(); err != nil {
		t.Fatalf("Close child: %v", err)
	}
	if err := parent.Close(); err != nil {
		t.Fatalf("Close parent: %v", err)
	}

	// The whole reason the prefix is copied rather than referenced.
	if err := m.Delete(parentID); err != nil {
		t.Fatalf("Delete parent: %v", err)
	}
	reopened, err := m.Open(childID, "test-model")
	if err != nil {
		t.Fatalf("open the child after deleting its parent: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if len(reopened.Messages()) != 2 {
		t.Errorf("child messages = %d, want its copied prefix", len(reopened.Messages()))
	}
}

func TestForkTakesTheBranchNotThePhysicalPrefix(t *testing.T) {
	m, parent := openManaged(t)
	seedTurns(t, parent)

	// Rewind, then continue: the parent now holds records that its own branch
	// does not reach.
	if _, err := parent.Rewind(RewindPoints(parent)[1].ItemID); err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	if err := parent.Append(llm.Message{Role: "user", Content: "the kept path"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	child, err := m.Fork(parent, RewindPoints(parent)[2].ItemID, "test-model")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	defer func() { _ = child.Close() }()

	for _, m := range child.Messages() {
		if m.Content == "second" {
			t.Fatalf("the child copied an abandoned branch: %#v", child.Messages())
		}
	}
	if got := child.Messages(); len(got) != 3 || got[2].Content != "the kept path" {
		t.Fatalf("child messages = %#v, want the live branch", got)
	}
}

func TestForkedChildStartsItsOwnUsageAndTraces(t *testing.T) {
	m, parent := openManaged(t)
	seedTurns(t, parent)
	parent.AddUsage(session.MetaUpdate{AddPromptTokens: 500, AddCompletionTokens: 100})

	child, err := m.Fork(parent, RewindPoints(parent)[1].ItemID, "test-model")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	defer func() { _ = child.Close() }()

	// Inheriting the parent's totals would double-count the same money the
	// moment anyone added the two sessions up.
	if child.PromptTokens() != 0 || child.CompletionTokens() != 0 {
		t.Errorf("child usage = %d/%d, want zero", child.PromptTokens(), child.CompletionTokens())
	}
	// Traces are per-run diagnostics, not resume input, so they are not copied.
	if entries, err := os.ReadDir(sessionstore.SessionTracesDir(m.Dir(), child.ID())); err == nil && len(entries) != 0 {
		t.Errorf("the child inherited traces: %v", entries)
	}
}

func TestForkCarriesTitleAndWorkspace(t *testing.T) {
	m, parent := openManaged(t)
	parent.SetTitle("a named conversation")
	parent.SetWorkspace("/repo")
	if err := parent.Append(llm.Message{Role: "user", Content: "one"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	child, err := m.Fork(parent, RewindPoints(parent)[0].ItemID, "test-model")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	defer func() { _ = child.Close() }()

	// A fork opens where the parent was, so it should not arrive untitled in
	// the default directory.
	if child.Title() != "a named conversation" {
		t.Errorf("title = %q", child.Title())
	}
	if child.Meta().Workspace != "/repo" {
		t.Errorf("workspace = %q", child.Meta().Workspace)
	}
}

func TestForkWritesAJournalTheLoaderAccepts(t *testing.T) {
	m, parent := openManaged(t)
	seedTurns(t, parent)

	points := RewindPoints(parent)
	child, err := m.Fork(parent, points[len(points)-1].ItemID, "test-model")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	childID := child.ID()
	if err := child.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read the bundle back through the plain loader, which validates the graph:
	// a fork that produced a journal the loader would refuse is worse than one
	// that failed outright.
	contents, err := sessionstore.Read(filepath.Join(m.Dir(), childID))
	if err != nil {
		t.Fatalf("the forked journal does not load: %v", err)
	}
	if contents.Header.SessionID != childID {
		t.Errorf("header names %q, want the child", contents.Header.SessionID)
	}
	// Renumbered from one: seq is a position in this journal, not the parent's.
	for i, it := range contents.Items {
		if it.Seq != uint64(i+1) {
			t.Fatalf("item %d has seq %d, want contiguous numbering from one", i, it.Seq)
		}
	}
	// Item ids are preserved, which is what lets a child's records be
	// recognised as the same work the parent did.
	parentIDs := map[string]bool{}
	for _, id := range parent.MessageIDs() {
		parentIDs[id] = true
	}
	for _, id := range child.MessageIDs() {
		if !parentIDs[id] {
			t.Errorf("child message id %s is not one of the parent's", id)
		}
	}
}

func TestForkRejectsAnUnknownTarget(t *testing.T) {
	m, parent := openManaged(t)
	seedTurns(t, parent)
	if _, err := m.Fork(parent, "no-such-item", "test-model"); err == nil {
		t.Fatal("Fork accepted an unknown target")
	}
}

func TestForkRejectsAnUnpersistedParent(t *testing.T) {
	m := NewSessionManager(t.TempDir())
	parent := NewSessionContext("test-model")
	if err := parent.Append(llm.Message{Role: "user", Content: "one"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Holding the parent's writer lock is what makes the branch stand still
	// while it is copied; without a parent on disk there is nothing to hold.
	if _, err := m.Fork(parent, "anything", "test-model"); err == nil {
		t.Fatal("Fork accepted a parent that is not open for writing")
	}
}

func TestForkedChildIsIndependentlyWritable(t *testing.T) {
	m, parent := openManaged(t)
	seedTurns(t, parent)

	child, err := m.Fork(parent, RewindPoints(parent)[1].ItemID, "test-model")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	defer func() { _ = child.Close() }()

	if err := child.Append(llm.Message{Role: "user", Content: "child goes its own way"}); err != nil {
		t.Fatalf("Append to child: %v", err)
	}
	if err := child.SetNotes([]agent.Note{{Text: "the child's note"}}, 1); err != nil {
		t.Fatalf("SetNotes on child: %v", err)
	}
	// Divergence is the point: neither session is now the other's history.
	if len(parent.Notes()) != 0 {
		t.Errorf("the child's note reached the parent: %+v", parent.Notes())
	}
	if len(parent.Messages()) != 5 {
		t.Errorf("parent messages = %d, want unchanged", len(parent.Messages()))
	}
}

func TestForkedChildIsListedAsItsOwnSession(t *testing.T) {
	m, parent := openManaged(t)
	seedTurns(t, parent)

	child, err := m.Fork(parent, RewindPoints(parent)[1].ItemID, "test-model")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	defer func() { _ = child.Close() }()

	rows, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want the parent and the child", len(rows))
	}
	// The picker carries the lineage, which is what lets a surface group forks
	// rather than showing two unrelated conversations with the same title.
	var found bool
	for _, r := range rows {
		if r.ID == child.ID() {
			found = true
			if r.ForkedFrom == nil || r.ForkedFrom.SessionID != parent.ID() {
				t.Errorf("child row lost its lineage: %+v", r.ForkedFrom)
			}
		}
	}
	if !found {
		t.Error("the child is not in the list")
	}
}

func TestHistoryPointsSkipAMessageThatAskedForTools(t *testing.T) {
	_, sess := openManaged(t)
	seedTurns(t, sess)

	// The branch ending at that message holds a call with no result. The
	// Anthropic adapter prunes the unanswered half; the OpenAI and Ollama ones
	// send it and the provider refuses the request, so it is not somewhere a
	// session may be left standing.
	for _, p := range RewindPoints(sess) {
		if p.Role == "assistant" && p.Content == "" {
			t.Fatalf("the picker offered a mid-turn message: %+v", p)
		}
	}
	if got := len(RewindPoints(sess)); got != 3 {
		t.Fatalf("points = %d, want the three messages that end where a turn ended", got)
	}
}
