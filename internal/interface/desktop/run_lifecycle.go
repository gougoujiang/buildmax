package desktop

import (
	"context"

	"github.com/icloudbb/buildmax/internal/agentapp"
	"github.com/icloudbb/buildmax/internal/core/agent"
)

// desktopRun adapts one scheduled run's lifecycle to the desktop's Wails events.
// A run's turn-by-turn progress reaches the frontend through these methods; the
// RunScheduler owns the serialization, the queue, and the session, so nothing
// here repeats that. Foreground prompts and job deliveries share this adapter
// and differ only in touchLastUsed and onStart.
type desktopRun struct {
	app       *App
	ctx       context.Context
	projectID string
	handler   agent.ApprovalHandler
	// touchLastUsed advances the project's recency stamp after each good turn.
	// A user prompt does; a background delivery does not, because the user did
	// not reach for the project.
	touchLastUsed bool
	// onStart announces the run before its first output, or is nil for a
	// foreground run, which has nothing to announce.
	onStart func(sess *agentapp.SessionContext)
}

func (r *desktopRun) RunOpts() agentapp.RunPromptOpts {
	return agentapp.RunPromptOpts{
		Stream:    &desktopStreamSink{ctx: r.ctx, emit: r.app.emit},
		Approval:  r.handler,
		EventSink: desktopEventSink(r.app.emit, r.ctx, func() []string { return r.app.scheduler.Queued(r.projectID) }),
		Digest:    true,
	}
}

func (r *desktopRun) OnStart(sess *agentapp.SessionContext) {
	if r.onStart != nil {
		r.onStart(sess)
	}
}

func (r *desktopRun) TurnDone(out agentapp.RunResult) {
	r.app.emitTurnDigest(r.ctx, out)
	if r.touchLastUsed {
		touchProjectLastUsed(r.projectID)
	}
}

func (r *desktopRun) TurnError(err error) {
	r.app.emit(r.ctx, eventStreamError, &StreamErrorPayload{Message: err.Error()})
}

func (r *desktopRun) Dequeued(next string, snapshot []string) {
	r.app.emit(r.ctx, eventMessageDequeued, &MessageDequeuedPayload{Prompt: next, Queued: snapshot})
}

func (r *desktopRun) Done(out agentapp.RunResult, err error) {
	if err != nil {
		r.app.emit(r.ctx, eventStreamError, &StreamErrorPayload{Message: err.Error()})
		return
	}
	r.app.emit(r.ctx, eventStreamDone, replyPayload(out))
}
