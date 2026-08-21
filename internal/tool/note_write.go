package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// NoteWrite records durable session notes. Unlike a tool result, a note is not a message: it
// survives history compaction and is re-rendered on every model call, so it is where the agent
// keeps what the conversation cannot be trusted to hold.
//
// The store is reached through the context rather than held on the tool, because the tool
// registry is cached per model and shared across sessions.
type NoteWrite struct{}

// NewNoteWrite creates a NoteWrite tool.
func NewNoteWrite() *NoteWrite { return &NoteWrite{} }

// Name returns the tool name for the LLM.
// Access implements llm.AccessDeclarer. Session notes have no lock, so this is a
// write and cannot run concurrently with a sibling call.
func (t *NoteWrite) Access(_ map[string]any) llm.Access { return llm.AccessWrite }

// DefaultAction implements llm.PolicyProvider, overriding the action the
// permission layer would otherwise derive from Access.
//
// What this writes is the agent's own scratch state, not anything the user
// owns. Asking a user to approve the agent taking a note would be noise, and
// the noise would arrive on every run. The write classification above is still
// the honest one — it is what keeps this tool out of a parallel batch.
// See docs/design/tool-permissions.md §5.2.
func (t *NoteWrite) DefaultAction() llm.ToolAction { return llm.ToolActionAllow }

func (t *NoteWrite) Name() string { return ToolNameNoteWrite }

// Description tells the LLM what belongs in a note. The behavioural contract matters more than
// the name here: without a stated rule the list fills with narration and duplicated context.
func (t *NoteWrite) Description() string {
	return "Record durable notes for this session. These notes survive compaction of the " +
		"conversation history and are shown to you on every turn. Record only what cannot be " +
		"recovered afterwards and that you will still need: decisions and the reasons for them, " +
		"approaches already ruled out, constraints the user stated once, facts about the " +
		"situation that later answers depend on. Do not record what re-reading a file would " +
		"recover, and do not narrate what you are currently doing. Pass the complete list; it " +
		"replaces the stored one."
}

// Parameters returns the OpenAI-style JSON schema for the tool arguments.
func (t *NoteWrite) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"notes": map[string]any{
				"type": "array",
				"description": fmt.Sprintf(
					"The complete note list, replacing the stored one. At most %d entries of %d characters each.",
					agent.MaxNotes, agent.MaxNoteChars),
				"items": map[string]any{
					"type":        "string",
					"description": "One note: a single line stating one durable fact or decision",
				},
			},
		},
		"required": []string{"notes"},
	}
}

// Execute replaces the session's notes with the given list.
func (t *NoteWrite) Execute(ctx context.Context, args map[string]any) (string, error) {
	v, ok := args["notes"]
	if !ok {
		return "", errors.New("notes is required")
	}
	slice, ok := v.([]any)
	if !ok {
		return "", errors.New("notes must be an array of strings")
	}

	texts := make([]string, 0, len(slice))
	for i, elem := range slice {
		s, ok := elem.(string)
		if !ok {
			return "", fmt.Errorf("notes[%d] must be a string", i)
		}
		texts = append(texts, strings.TrimSpace(s))
	}
	if err := agent.ValidateNotes(texts); err != nil {
		return "", err
	}

	store, ok := agent.NoteStoreFromContext(ctx)
	if !ok {
		// Say so plainly rather than reporting success: a note the model believes it stored and
		// which then vanishes is worse than no note at all.
		return "This run keeps no durable notes, so nothing was stored. Carry anything you need " +
			"forward in your reply instead.", nil
	}

	notes := make([]agent.Note, len(texts))
	for i, s := range texts {
		notes[i] = agent.Note{Text: s}
	}
	store.SetNotes(notes, agent.IterationFromContext(ctx))

	return formatNoteList(store.Notes()), nil
}

// formatNoteList echoes the stored notes so the model sees exactly what was kept.
func formatNoteList(notes []agent.Note) string {
	if len(notes) == 0 {
		return "Notes cleared. Nothing is stored for this session."
	}
	word := "notes"
	if len(notes) == 1 {
		word = "note"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Stored %d %s:\n", len(notes), word)
	for i, n := range notes {
		fmt.Fprintf(&b, "%d. %s\n", i+1, n.Text)
	}
	return strings.TrimSuffix(b.String(), "\n")
}
