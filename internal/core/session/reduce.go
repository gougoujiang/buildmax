package session

import (
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// State is what a branch of history reduces to: everything a resumed turn needs
// and nothing a turn produced only as evidence. Timing, usage, and sandbox
// details are the trace's business, not this.
type State struct {
	Messages          []llm.Message
	CompactionIdx     int
	CompactionSummary string
	Notes             []agent.Note
	Todos             []agent.Todo
	AdditionalPrompt  string
	// LastTurn is the turn id of the most recent TurnStarted on this branch,
	// and Open reports whether it lacks a matching TurnFinished. Together they
	// are what tells a caller whether it is resuming or repairing.
	LastTurn string
	Open     bool
}

// Reduce replays the branch ending at head and returns the state it produces.
//
// It is deterministic and total over a validated journal: the same items and
// head always give the same State, which is what lets an incremental reducer be
// checked against a full replay.
func Reduce(items []Item, head string) (State, error) {
	branch, err := Branch(items, head)
	if err != nil {
		return State{}, err
	}
	var st State
	// Tool results project to messages, so a call answered twice would silently
	// duplicate one. Recording which calls are already answered keeps the
	// projection idempotent without a second pass over the branch.
	answered := make(map[string]bool)
	for _, it := range branch {
		switch p := it.Payload.(type) {
		case TurnStarted:
			st.LastTurn = p.RunID
			st.Open = true
		case TurnFinished:
			st.Open = false
		case MessageItem:
			st.Messages = append(st.Messages, p.Message)
		case ToolResult:
			if answered[p.ToolCallID] {
				return State{}, fmt.Errorf("%w: tool call %s answered twice", ErrHistoryCorrupt, p.ToolCallID)
			}
			answered[p.ToolCallID] = true
			st.Messages = append(st.Messages, llm.Message{
				Role:       "tool",
				ToolCallID: p.ToolCallID,
				Content:    p.Content,
				Parts:      p.Parts,
			})
		case Compaction:
			idx, err := compactionBoundary(branch, p.CoveredHeadID)
			if err != nil {
				return State{}, err
			}
			st.CompactionIdx = idx
			st.CompactionSummary = p.Summary
		case NotesReplaced:
			st.Notes = append([]agent.Note(nil), p.Notes...)
		case TodosReplaced:
			st.Todos = append([]agent.Todo(nil), p.Todos...)
		case AdditionalPromptSet:
			st.AdditionalPrompt = p.Text
		case ToolExecutionStarted, HeadSelected, TurnRecovered:
			// Recorded for recovery, ordering, and provenance; none of them
			// changes the state a resumed turn starts from.
		case UnknownPayload:
			// Validate has already refused an unknown record marked required,
			// so reaching one here means it only added information.
		}
	}
	return st, nil
}

// compactionBoundary counts the messages a summary covers.
//
// The count is taken over the branch being reduced rather than over the whole
// journal, which is what keeps a summary from crossing into a branch that does
// not contain the messages it summarised.
func compactionBoundary(branch []Item, coveredHeadID string) (int, error) {
	if coveredHeadID == "" {
		return 0, fmt.Errorf("%w: compaction names no covered head", ErrHistoryCorrupt)
	}
	messages := 0
	for _, it := range branch {
		switch it.Payload.(type) {
		case MessageItem, ToolResult:
			messages++
		}
		if it.ID == coveredHeadID {
			return messages, nil
		}
	}
	return 0, fmt.Errorf("%w: compaction covers %s, which is not on this branch", ErrHistoryCorrupt, coveredHeadID)
}

// HistoryMessages returns the model-visible messages: everything after the
// compaction boundary, since what precedes it is represented by the summary.
func (s State) HistoryMessages() []llm.Message {
	if s.CompactionIdx > 0 && s.CompactionIdx <= len(s.Messages) {
		return s.Messages[s.CompactionIdx:]
	}
	return s.Messages
}
