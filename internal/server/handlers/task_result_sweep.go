package handlers

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// deliverySweepInterval is how often owed reports are retried.
//
// Frequent enough that a report delayed by a restart arrives while the person
// who asked for the work is plausibly still around, and rare enough that an
// idle deployment is doing one indexed query a minute.
const deliverySweepInterval = time.Minute

// deliverySweepLimit bounds one pass so a backlog cannot hold the loop or the
// database. The next tick continues.
const deliverySweepLimit = 50

// deliverySweeper retries the reports the server owes.
//
// A report is a Tier 1 turn, which is a model call: it can fail, be refused
// because the conversation's queue is full, or be interrupted by a restart
// between the run finishing and the turn starting. Without this loop, each of
// those means the conversation is simply never told, and nothing afterwards
// knows a report was owed.
//
// It is not what makes the result durable — the run is, and a task's card reads
// it directly. It makes the sentence about the result durable.
type deliverySweeper struct {
	handler  *Handler
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
	// cancel ends a pass already in progress. A pass writes Tier 1 turns, which
	// are model calls, so waiting for one to finish could outlast any shutdown
	// budget. Abandoning it costs nothing: the delivery is a row, and the next
	// process to run this loop finds it still due.
	cancel context.CancelFunc
}

// StartBackground launches the handler's background loops. Calling it more than
// once, or on a handler with no delivery store, is a no-op.
func (h *Handler) StartBackground() {
	if h == nil || h.cfg.TaskResultDeliveries == nil {
		return
	}
	h.sweepMu.Lock()
	defer h.sweepMu.Unlock()
	if h.sweeper != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &deliverySweeper{
		handler:  h,
		interval: deliverySweepInterval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		cancel:   cancel,
	}
	h.sweeper = s
	go s.loop(ctx)
	slog.Info("task result delivery sweeper started", "interval", s.interval)
}

// BeginDrain tells the parts of the API surface that own work of their own that
// this server is going away: no new conversation turn is admitted, and watcher
// streams see the closed drain channel the server passed in.
//
// It does not wait. WaitTurns is what gives a turn already running its moment.
func (h *Handler) BeginDrain() {
	if h == nil {
		return
	}
	h.turns.Drain()
}

// draining reports whether the server that owns this handler has begun stopping.
func (h *Handler) draining() bool {
	if h == nil || h.cfg.Drain == nil {
		return false
	}
	select {
	case <-h.cfg.Drain:
		return true
	default:
		return false
	}
}

// WaitTurns waits for the conversation turns already running, bounded by ctx.
//
// A turn reached over a WebSocket runs on a hijacked connection, and
// http.Server.Shutdown does not wait for those — so without this the answer
// being written to a user would disappear when the process exits.
func (h *Handler) WaitTurns(ctx context.Context) {
	if h == nil {
		return
	}
	if !h.turns.Wait(ctx) {
		slog.Warn("conversation turns did not finish before shutdown")
	}
}

// StopBackground stops the handler's background loops and waits for them, plus
// the terminal callbacks a finished run left running. Bounded by ctx: a
// shutdown that hangs here is worse than one that loses a callback.
func (h *Handler) StopBackground(ctx context.Context) {
	if h == nil {
		return
	}
	h.sweepMu.Lock()
	s := h.sweeper
	h.sweeper = nil
	h.sweepMu.Unlock()
	if s != nil {
		s.stopOnce.Do(func() { close(s.stopCh) })
		select {
		case <-s.doneCh:
		case <-ctx.Done():
			// A pass is still running. Abandon it rather than the rest of the
			// shutdown; the deliveries it did not get to stay due.
			s.cancel()
		}
		slog.Info("task result delivery sweeper stopped")
	}
	h.terminal.Wait(ctx)
}

func (s *deliverySweeper) loop(ctx context.Context) {
	defer close(s.doneCh)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	// One pass immediately: a restart is exactly when reports are owed, and
	// waiting a full interval to find out is the case this exists for.
	s.handler.SweepTaskResultDeliveries(ctx, time.Now())
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.handler.SweepTaskResultDeliveries(ctx, time.Now())
		}
	}
}

// SweepTaskResultDeliveries retries every report that is due. It is exported so
// a test can drive one pass without a clock.
func (h *Handler) SweepTaskResultDeliveries(ctx context.Context, now time.Time) {
	if h == nil || h.cfg.TaskResultDeliveries == nil {
		return
	}
	due, err := h.cfg.TaskResultDeliveries.ListDueTaskResultDeliveries(ctx, now, deliverySweepLimit)
	if err != nil {
		slog.Warn("task result delivery sweep failed", "err", err)
		return
	}
	for _, delivery := range due {
		h.attemptTaskResultDelivery(ctx, delivery.TaskRunID, now)
	}
}
