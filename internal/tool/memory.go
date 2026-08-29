package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
)

// The two memory tools. The index the model is shown carries a name and a
// one-clause description; MemoryRead opens the bodies worth opening, and
// MemoryWrite creates or replaces exactly one memory.
//
// Both reach the store through the context rather than holding it, because the
// tool registry is cached per model and shared across sessions.

// noMemoryStore is what a run without memory is told. Said plainly rather than
// reported as success: memory the model believes it stored and which then
// vanishes is worse than none.
const noMemoryStore = "This run keeps no project memory, so there is nothing to read or write. " +
	"Nothing recorded here will reach a later session."

// MemoryRead returns the bodies behind index lines.
type MemoryRead struct{}

// NewMemoryRead creates a MemoryRead tool.
func NewMemoryRead() *MemoryRead { return &MemoryRead{} }

func (t *MemoryRead) Name() string { return ToolNameMemoryRead }

// Access implements llm.AccessDeclarer. Reading changes nothing the user owns.
func (t *MemoryRead) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }

// DefaultAction implements llm.PolicyProvider. The content is the agent's own
// recall, kept under BUILDMAX_HOME, and a prompt to read it would arrive on
// every run for no decision the user could usefully make.
func (t *MemoryRead) DefaultAction() llm.ToolAction { return llm.ToolActionAllow }

func (t *MemoryRead) Description() string {
	return "Read what this project remembers. The project-memory block lists a name and a one-clause " +
		"description for each memory; those descriptions are pointers, not summaries, so open a memory " +
		"before relying on what its line suggests and before changing it. Reading is cheap and the " +
		"bodies carry the reason a memory is believed, which is the part that decides whether it still " +
		"applies. Names that do not exist are reported rather than failing the call."
}

func (t *MemoryRead) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"names": map[string]any{
				"type":        "array",
				"description": "The memory names to open, exactly as they appear in the project-memory block.",
				"items":       map[string]any{"type": "string"},
			},
		},
		"required": []string{"names"},
	}
}

// Execute returns the requested bodies.
func (t *MemoryRead) Execute(ctx context.Context, args map[string]any) (string, error) {
	names, err := stringSliceArg(args, "names")
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", errors.New("names is empty; pass the memory names to open")
	}
	store, ok := agent.MemoryStoreFromContext(ctx)
	if !ok {
		return noMemoryStore, nil
	}

	bodies, missing, err := store.Read(ctx, names)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i, m := range bodies {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## %s (%s)\n%s\n\n%s", m.Name, m.Type, m.Description, strings.TrimSpace(m.Body))
	}
	if len(missing) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		// Named rather than silently absent: a model that asked for a memory
		// and got nothing back must be able to tell "no such memory" from "the
		// body was empty".
		fmt.Fprintf(&b, "No memory named: %s.", strings.Join(missing, ", "))
	}
	if b.Len() == 0 {
		return "No memories matched.", nil
	}
	return b.String(), nil
}

// MemoryWrite creates or replaces exactly one memory.
//
// Unlike a note, this outlives the session. That is the whole value and the
// whole risk: a missed memory costs convenience, while a false or sensitive one
// misleads every future run in the project.
type MemoryWrite struct{}

// NewMemoryWrite creates a MemoryWrite tool.
func NewMemoryWrite() *MemoryWrite { return &MemoryWrite{} }

func (t *MemoryWrite) Name() string { return ToolNameMemoryWrite }

// Access implements llm.AccessDeclarer. Declaring it a write is what keeps two
// calls in one batch from racing each other into the store's lock.
func (t *MemoryWrite) Access(_ map[string]any) llm.Access { return llm.AccessWrite }

// DefaultAction implements llm.PolicyProvider.
//
// What this writes is the agent's own recall, kept under BUILDMAX_HOME and
// never in the user's repository, so a prompt on every call would be noise. The
// user's controls are the ones that matter and they are outside the model: the
// memories are theirs to read, edit, delete, or switch off for a run.
func (t *MemoryWrite) DefaultAction() llm.ToolAction { return llm.ToolActionAllow }

// Description is the behavioural contract. Without the "do not keep" half, the
// store fills with restated code and task narration, and every future session
// in the project pays for both.
func (t *MemoryWrite) Description() string {
	return "Record one thing this project should remember across sessions, or replace one it already " +
		"remembers. Every session in this project sees the name and description on every turn, " +
		"including sessions in other worktrees of the same repository.\n\n" +
		"Keep: stable preferences the user has stated, decisions and the reasons behind them, " +
		"corrections that have come up more than once, conventions that are not obvious from the tree, " +
		"and approaches already tried and ruled out. The test is that the fact stays true on any " +
		"branch — a conclusion that holds only inside the branch that reached it is session state, and " +
		"NoteWrite already keeps it where it applies.\n\n" +
		"Do not keep: anything a file or a command would answer cheaply, the state of the task you are " +
		"on now, narration, raw tool output, or credentials. Something expensive to reconstruct and slow " +
		"to change is worth a memory, but it must name its source of truth and tell the reader to verify " +
		"there before acting.\n\n" +
		"Recording a preference is not adopting a policy. A memory is recall you may be wrong about; it " +
		"never becomes a rule you cite as your own authority, and it loses to a current instruction or a " +
		"current user statement. When something should bind every future run, say so and let the user " +
		"put it in AGENTS.md — you cannot write that file and must not route around it here.\n\n" +
		"A feedback memory records what the user wants, never what the user is. \"Prefers the " +
		"recommendation before the survey\" is a preference. \"Is unfamiliar with X\" is a judgement " +
		"about a person, cannot be checked against anything, and will be acted on for months — do not " +
		"write one. An inference about the user needs repetition across sessions, and the body must " +
		"carry the occasions it rests on so the user can disagree with it on evidence.\n\n" +
		"The body states the fact, then **Why** it is believed, then **How to apply** it. To change an " +
		"existing memory, read it first: a replacement you have not read is refused. Empty content " +
		"deletes the memory."
}

func (t *MemoryWrite) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type": "string",
				"description": fmt.Sprintf(
					"The memory's identity and file name: lowercase letters, digits, and single hyphens, "+
						"at most %d characters. Reusing an existing name replaces that memory.",
					localproject.MaxMemoryNameChars),
			},
			"description": map[string]any{
				"type": "string",
				"description": fmt.Sprintf(
					"One line, at most %d characters, saying what this memory is about. It is the only "+
						"part shown on every turn, so it has to be worth its place and has to make clear "+
						"when opening the body would be useful.", localproject.MaxDescriptionChars),
			},
			"type": map[string]any{
				"type": "string",
				"enum": []string{
					string(localproject.MemoryTypeFeedback),
					string(localproject.MemoryTypeProject),
					string(localproject.MemoryTypeReference),
				},
				"description": "feedback: guidance the user gave about how to work here. " +
					"project: ongoing work, goals, decisions, and constraints. " +
					"reference: pointers to external resources.",
			},
			"content": map[string]any{
				"type": "string",
				"description": fmt.Sprintf(
					"The memory body in Markdown, at most %d characters: the fact, then **Why**, then "+
						"**How to apply**. An empty string deletes this memory.", localproject.MaxBodyChars),
			},
		},
		"required": []string{"name", "content"},
	}
}

// Execute writes or deletes one memory.
func (t *MemoryWrite) Execute(ctx context.Context, args map[string]any) (string, error) {
	name, err := parseRequiredStringRaw(args, "name")
	if err != nil {
		return "", err
	}
	// Not parseRequiredStringRaw: an empty body is the delete operation, so
	// "present but empty" has to be distinguishable from "missing".
	raw, ok := args["content"]
	if !ok {
		return "", errors.New("content is required")
	}
	content, ok := raw.(string)
	if !ok {
		return "", errors.New("content must be a string")
	}

	store, ok := agent.MemoryStoreFromContext(ctx)
	if !ok {
		return noMemoryStore, nil
	}

	if strings.TrimSpace(content) == "" {
		if err := store.Delete(ctx, name); err != nil {
			if errors.Is(err, localproject.ErrMemoryNotFound) {
				return fmt.Sprintf("There is no memory named %s, so nothing was deleted.", name), nil
			}
			return "", err
		}
		return fmt.Sprintf("Deleted the memory %s.", name), nil
	}

	written, err := store.Write(ctx, agent.MemoryUpsert{
		Name:        name,
		Description: parseOptionalString(args, "description", ""),
		Type:        parseOptionalString(args, "type", string(localproject.MemoryTypeProject)),
		Body:        content,
	})
	if err != nil {
		return "", memoryWriteError(name, err)
	}
	return fmt.Sprintf("Saved the memory %s (%s): %s", written.Name, written.Type, written.Description), nil
}

// memoryWriteError turns a refusal into something the model can act on. The
// two concurrency refusals differ in what to do next, so they do not share a
// message: one needs a read, the other needs a merge.
func memoryWriteError(name string, err error) error {
	switch {
	case errors.Is(err, localproject.ErrMemoryUnread):
		return fmt.Errorf("%s already exists and this run has not read it, so nothing was written. "+
			"Read it with %s, then write the merged version", name, ToolNameMemoryRead)
	case errors.Is(err, localproject.ErrMemoryConflict):
		return fmt.Errorf("%s changed since you read it — another session wrote it, or the user edited "+
			"the file — so nothing was written. Read it again and merge into what it now says", name)
	default:
		return err
	}
}

// stringSliceArg reads a required array-of-strings argument.
func stringSliceArg(args map[string]any, key string) ([]string, error) {
	v, ok := args[key]
	if !ok {
		return nil, fmt.Errorf("%s is required", key)
	}
	slice, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
	out := make([]string, 0, len(slice))
	for i, elem := range slice {
		s, ok := elem.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a string", key, i)
		}
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}
