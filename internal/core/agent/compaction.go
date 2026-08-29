package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// ErrCompactionNotPersisted wraps a durable history that refused to record a
// compaction. It is separated from every other compaction failure because only
// this one is fatal: the summary exists and the boundary does not, so the run
// cannot continue against a history that disagrees with what was summarized.
var ErrCompactionNotPersisted = errors.New("persist compaction")

// CompactResult reports what one compaction pass did.
//
// Summarized == 0 means nothing was replaced and Reason says why — an empty
// history, a hook that blocked, or nothing old enough to summarize. That is not
// an error: a compaction that finds no work to do leaves a usable session.
type CompactResult struct {
	// Summarized is how many model-visible messages the summary replaced.
	Summarized int
	// Kept is how many messages remain verbatim after the boundary.
	Kept int
	// Summary is what the summarizer produced, after clamping to its budget.
	Summary string
	// Reason explains a pass that compacted nothing. Empty when it did.
	Reason string
}

// Compacted reports whether the pass replaced anything.
func (r CompactResult) Compacted() bool { return r.Summarized > 0 }

// Compact compacts a history on demand, outside a run.
//
// It is the same pass RunLoop makes when the context window fills, minus the
// fill test: a user who asks for compaction is not asking whether it is due.
// Only the fields compaction itself uses are read from opts — History,
// Compactor, Checkpointer, Hooks, LLMClient, Pricing, EventSink, and the
// attribution fields hooks are given.
//
// The returned RunStats hold what the summarizing call spent, so a caller that
// keeps session totals can fold them in; they are reported even when the pass
// compacted nothing, because a call that produced an unusable summary was still
// paid for.
func Compact(ctx context.Context, opts RunLoopOpts) (CompactResult, RunStats, error) {
	var s RunStats
	if opts.History == nil {
		return CompactResult{}, s, errors.New("compact: no history")
	}
	if opts.Compactor == nil {
		return CompactResult{}, s, errors.New("compact: no compactor configured")
	}
	priorSummary := ""
	if ch, ok := opts.History.(CompactionHistory); ok {
		priorSummary = ch.PriorSummary()
	}
	history := opts.History.HistoryMessages()
	_, res, err := compactOnce(ctx, opts, &s, 0, priorSummary, history, manualReserveTokens(opts.LLMClient))
	return res, s, err
}

// manualReserveTokens is how much of the tail a manual compaction keeps verbatim.
// An unknown context window falls back to the fixed reserve the trim path uses.
func manualReserveTokens(client llm.LLMClient) int {
	cw := 0
	if client != nil {
		cw = client.ContextWindow()
	}
	if cw <= 0 {
		return defaultReserveTokens
	}
	return int(float64(cw) * manualCompactionReserve)
}

// compactOnce performs one compaction pass over history: the PreCompact hook,
// the state checkpoint, the summarizing call, the durable boundary, the event,
// and the PostCompact hook. It is the single implementation behind both the
// automatic pass and the manual one, so the two cannot drift on what a
// compaction is.
//
// iter is the loop iteration a run's pass belongs to, and 0 for a manual pass;
// it is only reported. It returns the messages the caller should continue with
// — history itself when nothing was compacted — and folds what the summarizing
// call spent into s.
//
// Errors: a durable commit that fails wraps ErrCompactionNotPersisted and is
// fatal to a run; a summarizer that fails is returned plain, and the caller may
// carry on with an uncompacted history.
func compactOnce(ctx context.Context, opts RunLoopOpts, s *RunStats, iter int, priorSummary string, history []llm.Message, reserveTokens int) ([]llm.Message, CompactResult, error) {
	toSummarize, toKeep := splitForCompaction(history, reserveTokens)
	if len(toSummarize) == 0 {
		return history, CompactResult{Kept: len(history), Reason: "nothing old enough to summarize"}, nil
	}

	pre := baseHookInput(opts, HookPreCompact)
	pre.Summarized = len(toSummarize)
	pre.Kept = len(toKeep)
	if preOut := runHook(ctx, opts.Hooks, pre); preOut.Blocked() {
		reason := preOut.Reason
		if reason == "" {
			reason = "blocked by a PreCompact hook"
		}
		slog.Info("context compaction skipped by hook", "iter", iter, "reason", reason)
		return history, CompactResult{Kept: len(history), Reason: reason}, nil
	}

	summary, usage, err := checkpointAndCompact(ctx, opts, iter, priorSummary, toSummarize)
	if err != nil {
		return history, CompactResult{Kept: len(history)}, err
	}
	cw := 0
	if opts.LLMClient != nil {
		cw = opts.LLMClient.ContextWindow()
	}
	limit := maxSummaryChars(cw)
	if clamped := clampSummary(summary, limit); clamped != summary {
		slog.Warn("compaction summary exceeded its budget, clamped", "iter", iter, "limit_chars", limit, "got_chars", len(summary))
		summary = clamped
	}
	if ch, ok := opts.History.(CompactionHistory); ok {
		// summarizedCount counts real history messages; the prior summary
		// prepended above is synthetic and never entered the history.
		if err := ch.AddCompaction(summary, len(toSummarize)); err != nil {
			return history, CompactResult{Kept: len(history)}, fmt.Errorf("%w: %w", ErrCompactionNotPersisted, err)
		}
	}
	slog.Info("context compacted", "iter", iter, "summarized", len(toSummarize), "kept", len(toKeep))

	// Priced here rather than at an llm_end: compaction is a call the run paid
	// for, but it is not a turn, and reporting it as one would put a reply in
	// the trace that the conversation never contained.
	callCost := s.addCall(usage, opts.Pricing)
	emit(opts.EventSink, Event{
		Kind:       EventContextCompacted,
		Iter:       iter,
		Summarized: len(toSummarize),
		Kept:       len(toKeep),
		CallUsage:  usage,
		CallCost:   callCost,
	})
	post := baseHookInput(opts, HookPostCompact)
	post.Summarized = len(toSummarize)
	post.Kept = len(toKeep)
	post.Summary = summary
	runHook(ctx, opts.Hooks, post)

	return toKeep, CompactResult{
		Summarized: len(toSummarize),
		Kept:       len(toKeep),
		Summary:    summary,
	}, nil
}
