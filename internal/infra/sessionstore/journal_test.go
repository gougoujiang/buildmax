package sessionstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

var testTime = time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)

func newJournal(t *testing.T) (string, *Journal) {
	t.Helper()
	dir := t.TempDir()
	j, err := Create(dir, session.NewHeader("s1", testTime))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	return dir, j
}

// chain builds items linked to each other, starting from parent.
func chain(parent string, payloads ...session.Payload) []session.Item {
	items := make([]session.Item, 0, len(payloads))
	for i, p := range payloads {
		id := "i" + string(rune('a'+i))
		items = append(items, session.NewItem(uint64(i+1), id, parent, testTime, "run1", p))
		parent = id
	}
	return items
}

func TestCreateWritesAReadableHeader(t *testing.T) {
	dir, j := newJournal(t)
	if err := j.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// A header-only journal has no items but is not corrupt: a session exists
	// from the moment it is created, before its first turn.
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Header.SessionID != "s1" || got.Header.Version != session.HistoryVersion {
		t.Errorf("header = %+v", got.Header)
	}
	if len(got.Items) != 0 {
		t.Errorf("items = %d, want 0", len(got.Items))
	}
}

func TestCreateRefusesAnExistingJournal(t *testing.T) {
	dir, j := newJournal(t)
	j.Close()
	// Adopting an existing file would let a bug that reuses a session id append
	// one conversation onto another.
	if _, err := Create(dir, session.NewHeader("s1", testTime)); err == nil {
		t.Fatal("Create overwrote an existing journal")
	}
}

func TestAppendRoundTripsItems(t *testing.T) {
	dir, j := newJournal(t)
	items := chain("",
		session.TurnStarted{RunID: "run1", Model: "anthropic/claude-opus", WorkspaceRoot: "/repo"},
		session.MessageItem{Message: llm.Message{Role: "user", Content: "hello"}},
		session.ToolExecutionStarted{ToolCallID: "call_1", ToolName: "Bash"},
		session.ToolResult{ToolCallID: "call_1", Status: session.ToolStatusCompleted, Content: "ok"},
		session.TurnFinished{Status: session.TurnCompleted},
	)
	if err := j.Append(items...); err != nil {
		t.Fatalf("Append: %v", err)
	}
	j.Close()

	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Items) != len(items) {
		t.Fatalf("items = %d, want %d", len(got.Items), len(items))
	}
	for i := range items {
		if got.Items[i].ID != items[i].ID || got.Items[i].Type() != items[i].Type() {
			t.Fatalf("item %d = %s/%s, want %s/%s", i,
				got.Items[i].ID, got.Items[i].Type(), items[i].ID, items[i].Type())
		}
	}
	// The state a resumed turn starts from must survive the file, which is the
	// only reason any of this is written down.
	head, err := session.Head(got.Items)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	st, err := session.Reduce(got.Items, head)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(st.Messages) != 2 || st.Messages[1].Role != "tool" {
		t.Errorf("reduced messages = %#v", st.Messages)
	}
}

func TestAppendIsDurableBeforeItReturns(t *testing.T) {
	dir, j := newJournal(t)
	items := chain("", session.MessageItem{Message: llm.Message{Role: "user", Content: "committed"}})
	if err := j.Append(items...); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Read through a separate handle while the writer is still open: nothing
	// that affects resume may be visible to the agent before the file has it.
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items = %d, want 1 already on disk", len(got.Items))
	}
}

func TestAppendKeepsMultilineContentOnOneLine(t *testing.T) {
	dir, j := newJournal(t)
	// One record per line is the whole framing, and tool output is full of
	// newlines. A record that split into two lines would read back as two
	// records or as one broken one.
	output := "first\nsecond\r\nthird"
	items := chain("",
		session.MessageItem{Message: llm.Message{Role: "assistant", Content: output}},
		session.ToolResult{ToolCallID: "call_1", Status: session.ToolStatusCompleted, Content: output},
	)
	if err := j.Append(items...); err != nil {
		t.Fatalf("Append: %v", err)
	}
	j.Close()

	raw, err := os.ReadFile(filepath.Join(dir, JournalFile))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if lines := countLines(raw); lines != 3 {
		t.Errorf("file has %d lines, want 3 (header plus two records)", lines)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(got.Items))
	}
	msg, ok := got.Items[0].Payload.(session.MessageItem)
	if !ok || msg.Message.Content != output {
		t.Errorf("content = %#v, want it preserved exactly", got.Items[0].Payload)
	}
}

func countLines(b []byte) int {
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

func TestReadReportsTornTailWithoutRepairingIt(t *testing.T) {
	dir := tornJournal(t)
	before, err := os.ReadFile(filepath.Join(dir, JournalFile))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items = %d, want the one complete record", len(got.Items))
	}
	if got.Truncated != 0 {
		t.Errorf("Read reported a repair it must not perform: %d", got.Truncated)
	}
	after, err := os.ReadFile(filepath.Join(dir, JournalFile))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	// Inspection never repairs: another process may still be appending here.
	if string(after) != string(before) {
		t.Error("Read modified the journal")
	}
}

func TestOpenAppendRepairsTornTailAndKeepsEarlierRecords(t *testing.T) {
	dir := tornJournal(t)
	j, got, err := OpenAppend(dir)
	if err != nil {
		t.Fatalf("OpenAppend: %v", err)
	}
	defer j.Close()
	if len(got.Items) != 1 || got.Items[0].ID != "ia" {
		t.Fatalf("items = %v, want the one complete record", got.Items)
	}
	if got.Truncated == 0 {
		t.Error("repair not reported")
	}

	// Appending after the repair must produce a journal that loads cleanly,
	// which is the point of cutting back to a record boundary.
	next := session.NewItem(2, "ib", "ia", testTime, "run2", session.MessageItem{
		Message: llm.Message{Role: "user", Content: "after repair"},
	})
	if err := j.Append(next); err != nil {
		t.Fatalf("Append: %v", err)
	}
	reread, err := Read(dir)
	if err != nil {
		t.Fatalf("Read after repair: %v", err)
	}
	if len(reread.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(reread.Items))
	}
}

// tornJournal writes one complete record and then a partial one, the shape an
// interrupted append leaves behind.
func tornJournal(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	j, err := Create(dir, session.NewHeader("s1", testTime))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	items := chain("", session.MessageItem{Message: llm.Message{Role: "user", Content: "complete"}})
	if err := j.Append(items...); err != nil {
		t.Fatalf("Append: %v", err)
	}
	j.Close()

	f, err := os.OpenFile(filepath.Join(dir, JournalFile), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := f.WriteString(`{"seq":2,"id":"ib","parent_id":"ia","ty`); err != nil {
		t.Fatalf("write partial: %v", err)
	}
	f.Close()
	return dir
}

func TestReadRejectsCorruptionBeforeTheTail(t *testing.T) {
	dir := corruptJournal(t)
	_, err := Read(dir)
	if !errors.Is(err, session.ErrHistoryCorrupt) {
		t.Fatalf("err = %v, want ErrHistoryCorrupt", err)
	}
	// The message has to name the file and the line, because a person is the
	// one who has to act on it.
	if msg := err.Error(); !contains(msg, JournalFile) || !contains(msg, "line 3") {
		t.Errorf("error does not locate the problem: %v", err)
	}
}

func TestOpenAppendRefusesCorruptionBeforeTheTail(t *testing.T) {
	dir := corruptJournal(t)
	if _, _, err := OpenAppend(dir); !errors.Is(err, session.ErrHistoryCorrupt) {
		t.Fatalf("err = %v, want ErrHistoryCorrupt", err)
	}
}

func TestSalvageRecoversThePrefixWithoutTouchingTheFile(t *testing.T) {
	dir := corruptJournal(t)
	before, err := os.ReadFile(filepath.Join(dir, JournalFile))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	got, err := Salvage(dir)
	if err != nil {
		t.Fatalf("Salvage: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "ia" {
		t.Fatalf("items = %v, want everything up to the first bad record", got.Items)
	}
	if got.Header.SessionID != "s1" {
		t.Errorf("header lost: %+v", got.Header)
	}
	after, err := os.ReadFile(filepath.Join(dir, JournalFile))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	// A damaged original is never rewritten in place; salvage offers a copy of
	// what was good and leaves the evidence alone.
	if string(after) != string(before) {
		t.Error("Salvage modified the journal")
	}
}

func TestSalvageStopsAtAGraphBreakToo(t *testing.T) {
	dir := t.TempDir()
	j, err := Create(dir, session.NewHeader("s1", testTime))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	good := chain("", session.MessageItem{Message: llm.Message{Role: "user", Content: "keep"}})
	if err := j.Append(good...); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Structurally valid JSON that breaks the graph: the parent never appears.
	orphan := session.NewItem(2, "ib", "ghost", testTime, "run1",
		session.MessageItem{Message: llm.Message{Role: "user", Content: "orphan"}})
	if err := j.Append(orphan); err != nil {
		t.Fatalf("Append: %v", err)
	}
	j.Close()

	if _, err := Read(dir); !errors.Is(err, session.ErrHistoryCorrupt) {
		t.Fatalf("Read err = %v, want ErrHistoryCorrupt", err)
	}
	got, err := Salvage(dir)
	if err != nil {
		t.Fatalf("Salvage: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "ia" {
		t.Fatalf("items = %v, want the valid prefix", got.Items)
	}
}

// corruptJournal writes a complete record, then a complete line that is not
// valid JSON, then another complete record after it.
func corruptJournal(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	j, err := Create(dir, session.NewHeader("s1", testTime))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	items := chain("", session.MessageItem{Message: llm.Message{Role: "user", Content: "keep"}})
	if err := j.Append(items...); err != nil {
		t.Fatalf("Append: %v", err)
	}
	j.Close()

	f, err := os.OpenFile(filepath.Join(dir, JournalFile), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := f.WriteString("not json at all\n{\"seq\":3,\"id\":\"ic\",\"parent_id\":\"ia\",\"type\":\"turn_finished\",\"required\":false,\"ts\":\"2026-08-24T10:00:00Z\",\"data\":{}}\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()
	return dir
}

func TestReadRejectsAnUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	line := `{"type":"history","version":999,"session_id":"s1","created_at":"2026-08-24T10:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, JournalFile), []byte(line), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := Read(dir); !errors.Is(err, session.ErrHistoryVersion) {
		t.Fatalf("err = %v, want ErrHistoryVersion", err)
	}
}

func TestReadRejectsAnIncompleteHeader(t *testing.T) {
	dir := t.TempDir()
	// A journal whose very first sync was interrupted has no header to repair
	// back to, so reporting it as an empty session would claim more than is known.
	if err := os.WriteFile(filepath.Join(dir, JournalFile), []byte(`{"type":"his`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := Read(dir); !errors.Is(err, session.ErrHistoryCorrupt) {
		t.Fatalf("err = %v, want ErrHistoryCorrupt", err)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
