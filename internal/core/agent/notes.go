package agent

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gougoujiang/buildmax/internal/util"
)

// Durable session state — notes and todos — is the second half of the rule the compaction
// work implements: state that must outlive the message list lives outside it, is re-rendered
// on every call, and is bounded by construction. A tool result is a message, so a list written
// by a tool is trimmed away like any other; state kept here is not.
//
// See docs/design/context-durability.md.

// Note is one durable session note.
type Note struct {
	Text string `json:"text"`
	// WrittenAt is the loop iteration at which this entry first appeared. Rewriting the list
	// with the same text does not reset it, so the value reflects the entry's age rather than
	// the age of the last write.
	WrittenAt int `json:"written_at,omitempty"`
}

// Todo statuses. These are the values a todo may carry; anything else is rejected.
const (
	TodoPending    = "pending"
	TodoInProgress = "in_progress"
	TodoCompleted  = "completed"
)

// Todo is one durable task-list entry.
type Todo struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"active_form,omitempty"`
	// WrittenAt is the loop iteration at which this entry last changed status. It is what
	// makes "in progress for 40 iterations" reportable.
	WrittenAt int `json:"written_at,omitempty"`
}

// Bounds on durable state. These are not tuning knobs: this state is rendered into every
// request and, unlike history, has no trimming path, so an unbounded list here is worse than
// a lost message.
const (
	// MaxNotes is the most notes a session may hold.
	MaxNotes = 15
	// MaxNoteChars bounds one note. Notes are one-liners; prose rots invisibly.
	MaxNoteChars = 200
	// MaxTodos is the most todos a session may hold.
	MaxTodos = 30
	// anchorBlockBudgetChars bounds the whole rendered block at roughly 800 tokens.
	anchorBlockBudgetChars = 3200
	// MaxInvariantChars bounds the restated invariants. They are repeated after the messages
	// on every call, so the section has to stay a reminder rather than a second copy of the
	// whole prompt.
	MaxInvariantChars = 1024
)

// invariantsHeading marks the section of an additional system prompt that is restated in the
// anchoring block. It is the only structural convention that text has: the rest of it is opaque
// and sits in the system prompt unparsed.
const invariantsHeading = "## Invariants"

// ExtractInvariants returns the body of the "## Invariants" section of an additional system
// prompt, or "" when it has none. The section is restated after the messages on every call,
// because an instruction present verbatim in the system prompt still loses ground as the context
// fills with tool output: storage keeps it, proximity keeps it followed.
func ExtractInvariants(systemPromptText string) string {
	idx := strings.Index(systemPromptText, invariantsHeading)
	if idx < 0 {
		return ""
	}
	body := systemPromptText[idx+len(invariantsHeading):]
	// Stop at the next heading of the same or higher level.
	for _, marker := range []string{"\n## ", "\n# "} {
		if end := strings.Index(body, marker); end >= 0 {
			body = body[:end]
		}
	}
	body = strings.TrimSpace(body)
	if utf8.RuneCountInString(body) > MaxInvariantChars {
		body = strings.TrimSpace(util.ClipRunes(body, MaxInvariantChars))
	}
	return body
}

// NoteStore is the write side of durable session state. Tools reach it through the context
// rather than through a constructor, because the tool registry is cached per model and shared
// across sessions — a tool holding a session pointer would leak one session's state into
// another.
type NoteStore interface {
	Notes() []Note
	// SetNotes replaces the stored notes. Entries whose text is unchanged keep their original
	// WrittenAt; iter stamps the rest.
	SetNotes(notes []Note, iter int)
	Todos() []Todo
	// SetTodos replaces the stored todos. Entries whose content and status are both unchanged
	// keep their original WrittenAt; iter stamps the rest, so a status change restarts the clock.
	SetTodos(todos []Todo, iter int)
}

// NotesHistory is an optional extension of MessageHistory implemented by histories that carry
// durable session state, mirroring how CompactionHistory extends it for the compaction
// boundary. RunLoop renders whatever it finds here after the message list.
type NotesHistory interface {
	MessageHistory
	NoteStore
}

type noteStoreKey struct{}

type iterationKey struct{}

// CtxWithNoteStore returns a context carrying the durable state store for the current run.
func CtxWithNoteStore(ctx context.Context, s NoteStore) context.Context {
	return context.WithValue(ctx, noteStoreKey{}, s)
}

// NoteStoreFromContext returns the store for the current run, or (nil, false) when the run
// has none — a subagent run, for instance, which keeps no durable state of its own.
func NoteStoreFromContext(ctx context.Context) (NoteStore, bool) {
	s, ok := ctx.Value(noteStoreKey{}).(NoteStore)
	return s, ok && s != nil
}

// CtxWithIteration returns a context carrying the loop iteration a tool is being called from.
func CtxWithIteration(ctx context.Context, iter int) context.Context {
	return context.WithValue(ctx, iterationKey{}, iter)
}

// IterationFromContext returns the loop iteration, or 0 when it is not known.
func IterationFromContext(ctx context.Context) int {
	iter, _ := ctx.Value(iterationKey{}).(int)
	return iter
}

// ValidateNotes checks a proposed note list against the bounds and returns an error written
// for the LLM: it names the limit and what to do, because a tool failure the model cannot act
// on is a dead end.
func ValidateNotes(texts []string) error {
	if len(texts) > MaxNotes {
		return fmt.Errorf("too many notes: %d given, limit is %d. Merge related entries or drop "+
			"the ones you no longer need, then call again with the complete list", len(texts), MaxNotes)
	}
	for i, t := range texts {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("note %d is empty; omit it instead", i+1)
		}
		if n := utf8.RuneCountInString(t); n > MaxNoteChars {
			return fmt.Errorf("note %d is %d characters, limit is %d. Notes are one-liners: "+
				"shorten it to the fact worth keeping", i+1, n, MaxNoteChars)
		}
	}
	return nil
}

// ValidateTodos checks a proposed todo list against the bounds.
func ValidateTodos(todos []Todo) error {
	if len(todos) > MaxTodos {
		return fmt.Errorf("too many todos: %d given, limit is %d. Drop completed entries and "+
			"call again with the complete list", len(todos), MaxTodos)
	}
	inProgress := 0
	for i, td := range todos {
		if strings.TrimSpace(td.Content) == "" {
			return fmt.Errorf("todo %d has empty content", i+1)
		}
		switch td.Status {
		case TodoPending, TodoInProgress, TodoCompleted:
		default:
			return fmt.Errorf("todo %d has status %q; valid values are %q, %q, %q",
				i+1, td.Status, TodoPending, TodoInProgress, TodoCompleted)
		}
		if td.Status == TodoInProgress {
			inProgress++
		}
	}
	if inProgress > 1 {
		return fmt.Errorf("%d todos are in_progress; exactly one task is in progress at a time", inProgress)
	}
	return nil
}

// StampNotes carries WrittenAt across a rewrite: an entry whose text already exists keeps the
// iteration it first appeared at, and everything else is stamped with iter.
func StampNotes(prev, next []Note, iter int) []Note {
	age := make(map[string]int, len(prev))
	for _, p := range prev {
		if _, seen := age[p.Text]; !seen {
			age[p.Text] = p.WrittenAt
		}
	}
	out := make([]Note, len(next))
	for i, n := range next {
		n.WrittenAt = iter
		if at, ok := age[n.Text]; ok {
			n.WrittenAt = at
		}
		out[i] = n
	}
	return out
}

// StampTodos carries WrittenAt across a rewrite. The key is content plus status, so moving an
// entry to in_progress restarts its clock — which is the number worth reporting.
func StampTodos(prev, next []Todo, iter int) []Todo {
	type key struct{ content, status string }
	age := make(map[key]int, len(prev))
	for _, p := range prev {
		k := key{p.Content, p.Status}
		if _, seen := age[k]; !seen {
			age[k] = p.WrittenAt
		}
	}
	out := make([]Todo, len(next))
	for i, td := range next {
		td.WrittenAt = iter
		if at, ok := age[key{td.Content, td.Status}]; ok {
			td.WrittenAt = at
		}
		out[i] = td
	}
	return out
}

// anchorEntry is one candidate line, carrying the priority that decides what survives when the
// block does not fit.
type anchorEntry struct {
	priority int
	section  int
	order    int
	text     string
}

// Sections of the anchoring block, in display order.
const (
	sectionInvariants = iota
	sectionNotes
	sectionTodo
)

// Priorities for the budget ladder, lowest number kept first. An in-progress task and the
// notes are what the model needs to stay on course; pending detail and completed history are
// what it can lose without losing the thread.
const (
	prioInvariant = iota
	prioInProgress
	prioNote
	prioPending
	prioCompleted
)

// RenderSessionState renders durable state as a block to be placed after the message list.
// Returns "" when there is nothing to say — a session with no invariants that never writes a
// note pays nothing.
//
// invariants is the restated hard-constraint section of the run's additional system prompt;
// pass "" when there is none. iter is the current loop iteration, used to report entry ages; pass 0 when unknown.
func RenderSessionState(invariants string, notes []Note, todos []Todo, iter int) string {
	var entries []anchorEntry
	add := func(prio, section int, text string) {
		entries = append(entries, anchorEntry{priority: prio, section: section, order: len(entries), text: text})
	}

	for _, line := range strings.Split(strings.TrimSpace(invariants), "\n") {
		if strings.TrimSpace(line) != "" {
			add(prioInvariant, sectionInvariants, strings.TrimRight(line, " \t"))
		}
	}

	for _, n := range notes {
		add(prioNote, sectionNotes, "- "+stamp(n.WrittenAt)+n.Text)
	}

	completed := 0
	for _, td := range todos {
		switch td.Status {
		case TodoInProgress:
			add(prioInProgress, sectionTodo, "- [in progress"+age(td.WrittenAt, iter)+"] "+td.Content)
		case TodoPending:
			add(prioPending, sectionTodo, "- [pending] "+td.Content)
		case TodoCompleted:
			completed++
		}
	}
	if completed > 0 {
		add(prioCompleted, sectionTodo, "- ("+strconv.Itoa(completed)+" completed)")
	}

	if len(entries) == 0 {
		return ""
	}

	kept, dropped := admit(entries, anchorBlockBudgetChars)
	if len(kept) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<session-state>")
	writeSection(&b, "Invariants", kept, sectionInvariants)
	writeSection(&b, "Notes", kept, sectionNotes)
	writeSection(&b, "Todo", kept, sectionTodo)
	if dropped > 0 {
		b.WriteString("\n\n(" + strconv.Itoa(dropped) + " lower-priority entries omitted to fit)")
	}
	b.WriteString("\n</session-state>")
	return b.String()
}

// admit keeps entries by priority until the budget runs out, then restores their original
// order so the block reads the way it was written.
func admit(entries []anchorEntry, budget int) (kept []anchorEntry, dropped int) {
	byPriority := make([]anchorEntry, len(entries))
	copy(byPriority, entries)
	sort.SliceStable(byPriority, func(i, j int) bool { return byPriority[i].priority < byPriority[j].priority })

	used := 0
	for _, e := range byPriority {
		cost := len(e.text) + 1
		if used+cost > budget {
			dropped++
			continue
		}
		used += cost
		kept = append(kept, e)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].order < kept[j].order })
	return kept, dropped
}

func writeSection(b *strings.Builder, title string, kept []anchorEntry, section int) {
	first := true
	for _, e := range kept {
		if e.section != section {
			continue
		}
		if first {
			b.WriteString("\n\n## " + title + "\n")
			first = false
		} else {
			b.WriteString("\n")
		}
		b.WriteString(e.text)
	}
}

// stamp renders a note's age marker, or "" when the iteration is unknown.
func stamp(writtenAt int) string {
	if writtenAt <= 0 {
		return ""
	}
	return "[i" + strconv.Itoa(writtenAt) + "] "
}

// age renders how long a todo has held its current status, when that is known and non-trivial.
func age(writtenAt, iter int) string {
	if writtenAt <= 0 || iter <= writtenAt {
		return ""
	}
	return ", " + strconv.Itoa(iter-writtenAt) + " iterations"
}
