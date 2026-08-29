package agent

import (
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/util"
)

const defaultReserveTokens = 4096

// compactionThreshold is the fraction of the context window that triggers compaction.
// At 0.80 we still have headroom for the compaction LLM call output and the next real call.
const compactionThreshold = 0.80

// compactionReserve is the fraction of the context window kept verbatim after compaction.
// 0.20 leaves ~55% runway before the next compaction fires, reducing compaction frequency.
const compactionReserve = 0.20

// compactionSummaryBudget is the fraction of the context window the stored compaction
// summary may occupy. The summary lives in the system prompt, which is re-sent in full on
// every call and is never trimmed, so it needs a ceiling that history does not need.
const compactionSummaryBudget = 0.02

// defaultMaxSummaryChars bounds the summary when the context window is unknown.
const defaultMaxSummaryChars = 8000

// summaryClampMarker is appended when a summary had to be cut to fit its budget, so the model
// is told the block is incomplete rather than reading a sentence that simply stops.
const summaryClampMarker = "\n\n[earlier detail dropped: summary exceeded its budget]"

// priorSummaryPreamble labels the previous summary when it is fed back into the next
// compaction. It tells the summarizing model that this block is itself a summary of
// even older material, not part of the conversation being summarized.
const priorSummaryPreamble = "Summary of the conversation up to this point, produced by an " +
	"earlier compaction. Carry its still-relevant content forward into your new summary:"

// EstimateMessageTokens returns a character-based token estimate for one message.
// Uses the standard 4-chars-per-token heuristic plus 4 tokens of overhead per message for JSON framing.
func EstimateMessageTokens(m llm.Message) int {
	chars := len(m.Role) + len(m.Content) + len(m.ToolCallID)
	for _, tc := range m.ToolCalls {
		chars += len(tc.ID) + len(tc.Name) + len(tc.Arguments)
	}
	return chars/4 + 4
}

// EstimateTokens returns the total estimated token count for a slice of messages.
func EstimateTokens(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateMessageTokens(m)
	}
	return total
}

// splitForCompaction splits msgs into (toSummarize, toKeep) such that toKeep fits within
// reserveTokens. The split is always at a clean group boundary — never mid tool-call group.
// Returns (nil, msgs) when there is nothing old enough to summarize.
func splitForCompaction(msgs []llm.Message, reserveTokens int) (toSummarize, toKeep []llm.Message) {
	if len(msgs) == 0 {
		return nil, msgs
	}

	// Walk from newest to oldest, accumulating cost until we've reserved enough tokens.
	cost := 0
	keepFrom := len(msgs) // all messages start as "to summarize"
	for i := len(msgs) - 1; i >= 0; i-- {
		cost += EstimateMessageTokens(msgs[i])
		if cost >= reserveTokens {
			keepFrom = i
			break
		}
	}

	if keepFrom == 0 {
		// Everything fits inside the reserve — nothing to summarize.
		return nil, msgs
	}

	// Advance keepFrom to a clean group boundary: never start on a tool-role message,
	// because the paired assistant tool-call message must precede it.
	for keepFrom < len(msgs) && msgs[keepFrom].Role == "tool" {
		keepFrom++
	}

	if keepFrom >= len(msgs) {
		return nil, msgs
	}

	return msgs[:keepFrom], msgs[keepFrom:]
}

// TrimHistory returns a suffix of messages that fits within the token budget:
//
//	budget = contextWindow - reserveTokens - systemTokens
//
// Tool-call groups are kept or dropped as a unit: an assistant message with ToolCalls
// and its paired tool-role messages are never split, because most LLM APIs reject
// sequences where one side of a tool call is missing.
//
// When reserveTokens is 0, defaultReserveTokens (4096) is used.
// When the budget is too small to keep any messages, the most recent message is kept.
func TrimHistory(msgs []llm.Message, systemTokens, contextWindow, reserveTokens int) []llm.Message {
	if reserveTokens == 0 {
		reserveTokens = defaultReserveTokens
	}
	budget := contextWindow - reserveTokens - systemTokens
	if budget <= 0 || len(msgs) == 0 {
		return msgs
	}

	// Walk from newest to oldest, accumulating cost until we exceed the budget.
	cost := 0
	cutAt := len(msgs) // index of oldest message to keep (exclusive lower bound → keep msgs[cutAt:])
	for i := len(msgs) - 1; i >= 0; i-- {
		cost += EstimateMessageTokens(msgs[i])
		if cost > budget {
			break
		}
		cutAt = i
	}

	if cutAt == 0 {
		return msgs // everything fits
	}

	// Advance cutAt to a safe boundary so we never split a tool-call group.
	// A tool-role message must always be preceded by its assistant message.
	// Walk forward from cutAt until the first message is not a tool-role message.
	for cutAt < len(msgs) && msgs[cutAt].Role == "tool" {
		cutAt++
	}

	// Ensure at least one message is kept.
	if cutAt >= len(msgs) {
		cutAt = len(msgs) - 1
	}

	dropped := cutAt
	slog.Warn("context window trim", "dropped_messages", dropped, "kept_messages", len(msgs)-dropped)
	return msgs[cutAt:]
}

// maxSummaryChars returns the character ceiling for a stored compaction summary.
// contextWindow is in tokens; the 4-chars-per-token heuristic matches EstimateMessageTokens.
func maxSummaryChars(contextWindow int) int {
	if contextWindow <= 0 {
		return defaultMaxSummaryChars
	}
	return int(float64(contextWindow)*compactionSummaryBudget) * 4
}

// withPriorSummary prepends the previous compaction summary to msgs as a synthetic user
// message, so compaction N summarizes (summary N-1 + newly discarded messages) rather than
// the discarded messages alone.
//
// Without this the stored summary is replaced outright on every compaction and everything
// the earlier one covered is lost — not summarized, gone. Returns msgs unchanged when there
// is no prior summary.
func withPriorSummary(prior string, msgs []llm.Message) []llm.Message {
	if prior == "" {
		return msgs
	}
	out := make([]llm.Message, 0, len(msgs)+1)
	out = append(out, llm.Message{Role: "user", Content: priorSummaryPreamble + "\n\n" + prior})
	return append(out, msgs...)
}

// clampSummary bounds a summary to maxChars, preferring to cut at a line boundary so the
// result does not end mid-bullet. A summary that needs clamping means the compactor ignored
// its budget, so the caller is expected to log it.
func clampSummary(s string, maxChars int) string {
	if maxChars <= 0 || utf8.RuneCountInString(s) <= maxChars {
		return s
	}
	clipped := util.ClipRunes(s, maxChars)
	// Back off to the last line boundary when that keeps most of the budget.
	if cut := strings.LastIndexByte(clipped, '\n'); cut > maxChars*4/5 {
		clipped = clipped[:cut]
	}
	return strings.TrimRight(clipped, " \t\n") + summaryClampMarker
}

// RenderCompactionBlock formats a compaction summary for injection into the system prompt.
// It is the single definition of that block so every caller produces the same shape and no
// two callers can append competing copies.
func RenderCompactionBlock(summary string) string {
	if summary == "" {
		return ""
	}
	return "\n\n<context_compaction>\n" + summary + "\n</context_compaction>"
}

// PromptLayer names one contributor to a run system prompt and how large it was.
//
// The layers are what the agent was told before the conversation started, and trust-harness
// section 3.6 requires a run to be able to say which of them it loaded. That visibility is also
// what makes last-writer-wins safe for the additional system prompt: an identity change is
// observable
// afterwards rather than something an error has to prevent up front.
type PromptLayer struct {
	Name  string `json:"name"`
	Chars int    `json:"chars"`
}

// ContextSources is everything a run was given before its first model call, and
// where each part came from.
//
// It exists because "memory" had been used for four different things -- an
// instruction layer, a compaction summary, session notes, a shared document --
// and a diagnostic that called them all by one name could not answer which of
// them put a line in front of the model. Each source keeps its own kind here.
//
// No raw text. The session bundle and the Project bundle already hold the
// content, trace redaction is fail-open, and a diagnostic that copied
// instructions and memory into a third file would widen the blast radius of
// every one of them to answer a question sizes and revisions already answer.
type ContextSources struct {
	// ProjectID names the scope shared memory belongs to, empty for a run that
	// has none.
	ProjectID string `json:"project_id,omitempty"`
	// Workspace is the root this run executed against, which for a Project with
	// several worktrees is not derivable from ProjectID.
	Workspace string `json:"workspace,omitempty"`
	// Instructions are the system-prompt layers, in order. Written even when
	// there is nothing beyond the runtime prompt: an absent list would read as
	// "nobody looked" rather than "there was nothing else".
	Instructions []PromptLayer `json:"instructions,omitempty"`
	// Memory is every fallible recall source this run loaded.
	Memory []MemorySourceInfo `json:"memory,omitempty"`
	// HistoryProjection describes what stood in for messages the run no longer
	// holds in full.
	HistoryProjection HistoryProjection `json:"history_projection"`
}

// MemorySourceInfo identifies one memory source without quoting it. Revision
// and Digest are set for a versioned document; Entries for a counted list.
type MemorySourceInfo struct {
	Name     string `json:"name"`
	Revision int    `json:"revision,omitempty"`
	Digest   string `json:"digest,omitempty"`
	Chars    int    `json:"chars,omitempty"`
	Entries  int    `json:"entries,omitempty"`
}

// HistoryProjection reports whether a compaction summary stood in for messages
// this run no longer holds, and how large it was. The journal remains the
// authority on what actually happened; this says only that the model was
// reading a lossy view of it.
type HistoryProjection struct {
	CompactionPresent bool `json:"compaction_present"`
	Chars             int  `json:"chars,omitempty"`
}
