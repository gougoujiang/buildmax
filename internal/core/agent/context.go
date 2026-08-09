package agent

import (
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

const defaultReserveTokens = 4096

// compactionThreshold is the fraction of the context window that triggers compaction.
// At 0.75 we still have headroom for the compaction LLM call output and the next real call.
const compactionThreshold = 0.80

// compactionReserve is the fraction of the context window kept verbatim after compaction.
// 0.20 leaves ~55% runway before the next compaction fires, reducing compaction frequency.
const compactionReserve = 0.20

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
