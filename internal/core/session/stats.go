package session

import (
	"sort"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// ConversationStats is the shape of a session's history: who said how much,
// which tools ran, and how many bytes each of them put back into the context.
//
// It is derived from the stored messages alone and needs no run to be live, so
// it answers for a session whose traces have been removed. What it cannot
// answer is anything time-shaped — durations, run boundaries, which model ran
// — because the history carries no timestamps. That half comes from the traces.
type ConversationStats struct {
	// UserMessages counts messages the user actually wrote. A background
	// event travels as a user-role message because no provider has a portable
	// role for one, and counting those as things the user said would inflate
	// the number that reads most like effort.
	UserMessages int
	// BackgroundMessages counts the user-role messages that carry a Source:
	// command results, subagent results, monitor events.
	BackgroundMessages int
	// AssistantTurns counts assistant messages, including the ones whose whole
	// content was a tool call.
	AssistantTurns int
	// ToolCalls counts calls the assistant issued. ToolResults counts the
	// results that came back; the two differ when a run was cut short between
	// the call and its result.
	ToolCalls   int
	ToolResults int
	// Tools is the per-tool breakdown, heaviest by result bytes first. A tool
	// the assistant called but whose result never arrived still appears, with
	// zero bytes.
	Tools []ToolStats
	// TextBytes is what the conversation's own text weighs — user prompts and
	// assistant replies. ToolResultBytes is what came back from tools.
	//
	// Kept apart because they are spent differently: the first is the
	// conversation, the second is what the run pulled into it, and on a long
	// agent session the second is usually the larger by an order of magnitude.
	TextBytes       int
	ToolResultBytes int
	// CompactedMessages is how many messages sit before the compaction
	// boundary — summarized away, still stored.
	CompactedMessages int
	// Notes and Todos are the durable state the session carries.
	Notes int
	Todos int
}

// ToolStats is one tool's share of a session.
type ToolStats struct {
	Name string
	// Calls is how many times the assistant asked for it.
	Calls int
	// ResultBytes is what its results put back into the context. This is the
	// number that answers which tool is filling the context window, and it is
	// not derivable from the call count: one search can outweigh fifty reads.
	ResultBytes int
	// MaxResultBytes is the largest single result, so one outlier is not
	// hidden inside an average.
	MaxResultBytes int
}

// Stats folds a session's stored history into its shape.
//
// A tool result names the call it answers rather than the tool it came from,
// so results are attributed by walking the assistant tool calls first and
// looking each result's call id up. A result whose call is not in the history
// — the assistant message was compacted out from under it — is counted in the
// totals under an empty name rather than dropped: the bytes are in the context
// either way.
func Stats(s *Session) ConversationStats {
	if s == nil {
		return ConversationStats{}
	}
	var out ConversationStats
	out.CompactedMessages = min(s.CompactionIdx, len(s.Messages))
	out.Notes = len(s.NoteEntries)
	out.Todos = len(s.TodoEntries)

	byCall := make(map[string]string)
	byTool := make(map[string]*ToolStats)
	tool := func(name string) *ToolStats {
		t, ok := byTool[name]
		if !ok {
			t = &ToolStats{Name: name}
			byTool[name] = t
		}
		return t
	}

	for _, m := range s.Messages {
		switch m.Role {
		case "user":
			if m.Source == "" {
				out.UserMessages++
			} else {
				out.BackgroundMessages++
			}
			out.TextBytes += len(m.Content)
		case "assistant":
			out.AssistantTurns++
			out.TextBytes += len(m.Content)
			for _, tc := range m.ToolCalls {
				out.ToolCalls++
				byCall[tc.ID] = tc.Name
				tool(tc.Name).Calls++
			}
		case "tool":
			out.ToolResults++
			out.ToolResultBytes += len(m.Content)
			t := tool(byCall[m.ToolCallID])
			t.ResultBytes += len(m.Content)
			if n := len(m.Content); n > t.MaxResultBytes {
				t.MaxResultBytes = n
			}
		}
	}

	out.Tools = make([]ToolStats, 0, len(byTool))
	for _, t := range byTool {
		out.Tools = append(out.Tools, *t)
	}
	sort.Slice(out.Tools, func(i, j int) bool {
		a, b := out.Tools[i], out.Tools[j]
		if a.ResultBytes != b.ResultBytes {
			return a.ResultBytes > b.ResultBytes
		}
		if a.Calls != b.Calls {
			return a.Calls > b.Calls
		}
		return a.Name < b.Name
	})
	return out
}

// CacheReadShare is the fraction of the session's prompt that was served from
// a provider's cache, and ok=false when there is nothing to divide or the
// provider reported no cache usage at all. A zero share and an unreported one
// are different facts: only the first says the cache missed.
func (s *Session) CacheReadShare() (share float64, ok bool) {
	if s == nil || s.PromptTokens <= 0 {
		return 0, false
	}
	if s.CacheReadTokens == 0 && s.CacheWriteTokens == 0 {
		return 0, false
	}
	return float64(s.CacheReadTokens) / float64(s.PromptTokens), true
}

// Usage is the session's accumulated token usage.
func (s *Session) Usage() llm.Usage {
	if s == nil {
		return llm.Usage{}
	}
	return llm.Usage{
		PromptTokens:     s.PromptTokens,
		CompletionTokens: s.CompletionTokens,
		TotalTokens:      s.PromptTokens + s.CompletionTokens,
		CacheReadTokens:  s.CacheReadTokens,
		CacheWriteTokens: s.CacheWriteTokens,
	}
}
