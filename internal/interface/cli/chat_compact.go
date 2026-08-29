package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"

	tea "charm.land/bubbletea/v2"
)

// compactDoneMsg is sent when a compaction the user asked for finishes.
type compactDoneMsg struct {
	Result agentapp.CompactResult
	Err    error
}

// runSlashCompact summarizes the conversation so far and continues from the
// summary. It goes through the busy state a turn uses because it is one model
// call the user is waiting on, and because the session takes the same lock: a
// prompt typed meanwhile is queued and runs against the compacted history.
func runSlashCompact(m *Model) (tea.Model, tea.Cmd) {
	if m.opts.App == nil || m.opts.Session == nil {
		m.err = "no session is open"
		return m, nil
	}
	channel := beginRun(m)
	m.busyLabel = "Compacting"
	opts := m.opts
	started := m.runs.Go(func(ctx context.Context) {
		defer close(channel)
		res, err := opts.App.CompactSession(ctx, opts.Session)
		sendTUIMessage(ctx, channel, compactDoneMsg{Result: res, Err: err})
	})
	if !started {
		close(channel)
		m.busy = false
		m.busyLabel = ""
		m.streamChannel = nil
		m.err = "the TUI is shutting down"
		return m, nil
	}
	return m, tea.Batch(
		tea.Tick(time.Duration(carouselTick)*time.Millisecond, func(t time.Time) tea.Msg { return carouselTickMsg{} }),
		func() tea.Msg { return <-channel },
	)
}

func handleCompactDone(m *Model, msg compactDoneMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	m.busyLabel = ""
	m.carouselDots = 0
	m.streamChannel = nil
	if msg.Err != nil {
		m.err = msg.Err.Error()
		return m, drainQueueCmd()
	}
	m.runStatus = msg.Result.Status
	// The summary answers questions the last turn asked as well as the model
	// now can; the suggestion was written against messages that are gone.
	m.dropStaleSuggestion()
	banner := messageBarStyle.Render("─── Compacted ───")
	return m, tea.Sequence(
		tea.Println(banner+"\n\n"+renderCompacted(msg.Result)),
		drainQueueCmd(),
	)
}

// renderCompacted is what the user is told after a compaction.
//
// It reports the messages, not just the tokens: what the model can still quote
// verbatim is the part that changed, and a line about freed context alone would
// leave the user believing the conversation is intact.
func renderCompacted(r agentapp.CompactResult) string {
	if r.Summarized == 0 {
		reason := r.Reason
		if reason == "" {
			reason = "there was nothing old enough to summarize"
		}
		return "Nothing was compacted: " + reason + "."
	}
	line := fmt.Sprintf("%s summarized into the context block, %s kept verbatim.",
		plural(r.Summarized, "message", "messages"), plural(r.Kept, "message", "messages"))
	if r.BeforeTokens > 0 && r.Status.ContextTokens > 0 {
		line += fmt.Sprintf("\nContext is about %s tokens, down from %s.",
			formatCount(r.Status.ContextTokens), formatCount(r.BeforeTokens))
	}
	return line + "\nThe agent reads the summary from here on; earlier detail is only in the session file."
}
