package agentapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	tools "github.com/gougoujiang/buildmax/internal/tool"
)

// maxCheckpointTurns bounds the checkpoint conversation. Two rather than one so a write that
// fails validation — an over-long note list, say — can be corrected: the whole point of the
// checkpoint is that this is the last moment the material exists, so losing it to a rejected
// tool call would defeat the exercise. It never runs longer than that.
const maxCheckpointTurns = 2

const checkpointSystemPrompt = `You are updating the durable notes for an agent session.

The transcript below is about to be removed from the conversation to free context. It will be
replaced by a short summary that cannot carry detail. Anything in it you will still need must be
in your notes now — this is the last moment the material exists.

Record only what will not be recoverable afterwards: decisions and the reasons behind them,
approaches already tried and ruled out, constraints stated once, facts that later work depends
on. Do not record what re-reading a file would recover, and do not narrate what was done.

Call NoteWrite with the complete note list, and TodoWrite if the task list changed; each call
replaces what is stored, so carry forward the existing entries you still want. If nothing in the
transcript needs keeping, reply with no tool calls.`

// NoteCheckpointer implements agent.StateCheckpointer. Before a compaction discards messages, it
// gives the model one bounded turn to move what matters into durable session state.
//
// It is a separate model call rather than a job handed to the summarizer: the summarizer is
// answering "what happened", which is a different question from "what will I still need", and it
// answers it from a context that does not include the run's own notes.
type NoteCheckpointer struct {
	client llm.LLMClient
	tools  []llm.Tool
}

// NewNoteCheckpointer creates a checkpointer backed by the given LLM client. Its tool set is
// deliberately just the two state-writing tools: with a file or shell tool in reach the model
// treats the checkpoint as a turn to keep working.
func NewNoteCheckpointer(client llm.LLMClient) *NoteCheckpointer {
	return &NoteCheckpointer{
		client: client,
		tools:  []llm.Tool{tools.NewNoteWrite(), tools.NewTodoWrite()},
	}
}

// Checkpoint runs the checkpoint turn. It is a no-op when the run keeps no durable state or when
// there is nothing to look at.
func (c *NoteCheckpointer) Checkpoint(ctx context.Context, discarded []llm.Message) error {
	if c == nil || c.client == nil || len(discarded) == 0 {
		return nil
	}
	store, ok := agent.NoteStoreFromContext(ctx)
	if !ok {
		return nil // nowhere to write; nothing to do
	}

	defs := make([]llm.ToolDef, 0, len(c.tools))
	byName := make(map[string]llm.Tool, len(c.tools))
	for _, t := range c.tools {
		defs = append(defs, llm.ToolDef{Name: t.Name(), Description: t.Description(), Parameters: t.Parameters()})
		byName[t.Name()] = t
	}

	messages := []llm.Message{{Role: "system", Content: checkpointSystemPrompt}}
	if live := agent.RenderSessionState("", store.Notes(), store.Todos()); live != "" {
		messages = append(messages, llm.Message{Role: "user", Content: "What you have stored so far:\n\n" + live})
	}
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: "Transcript about to be discarded:\n\n" + transcript(discarded),
	})

	wrote := 0
	for turn := 0; turn < maxCheckpointTurns; turn++ {
		completion, err := c.client.ChatCompletionBlocking(ctx, messages, defs)
		if err != nil {
			return fmt.Errorf("checkpoint call: %w", err)
		}
		toolCalls := completion.ToolCalls
		if len(toolCalls) == 0 {
			break
		}
		// The checkpoint's own conversation is replayed to the same client, so
		// it carries reasoning state exactly as the main loop does.
		messages = append(messages, completion.AssistantMessage())

		failed := false
		for _, tc := range toolCalls {
			result, terr := runCheckpointTool(ctx, byName, tc)
			if terr != nil {
				failed = true
				result = "error: " + terr.Error()
				slog.Warn("checkpoint tool call rejected", "tool", tc.Name, "err", terr)
			} else {
				wrote++
			}
			messages = append(messages, llm.Message{Role: "tool", Content: result, ToolCallID: tc.ID})
		}
		if !failed {
			break // the model wrote what it wanted; a second turn would only invite more
		}
	}

	slog.Info("state checkpoint before compaction", "discarded_messages", len(discarded), "writes", wrote)
	return nil
}

// runCheckpointTool decodes one tool call and executes it against the session's store.
func runCheckpointTool(ctx context.Context, byName map[string]llm.Tool, tc llm.ToolCall) (string, error) {
	tool, ok := byName[tc.Name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q; only NoteWrite and TodoWrite are available here", tc.Name)
	}
	args := map[string]any{}
	if tc.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	return tool.Execute(ctx, args)
}

// transcript flattens messages into plain text.
//
// The discarded slice contains assistant tool calls and their results, and the checkpoint offers
// a tool set those calls do not belong to. Replaying them as structured messages would ask the
// provider to accept tool calls naming tools that are not on offer; a transcript sidesteps that
// entirely and reads the way the question is posed — as a record to review, not a conversation
// to continue.
func transcript(msgs []llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		switch {
		case len(m.ToolCalls) > 0:
			if strings.TrimSpace(m.Content) != "" {
				fmt.Fprintf(&b, "[assistant] %s\n", m.Content)
			}
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "[assistant calls %s] %s\n", tc.Name, tc.Arguments)
			}
		case m.Role == "tool":
			fmt.Fprintf(&b, "[tool result] %s\n", m.Content)
		default:
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			fmt.Fprintf(&b, "[%s] %s\n", m.Role, m.Content)
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}
