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
	s := &deliverySweeper{
		handler:  h,
		interval: deliverySweepInterval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
	h.sweeper = s
	go s.loop()
	slog.Info("task result delivery sweeper started", "interval", s.interval)
}

// StopBackground stops the handler's background loops and waits for them.
func (h *Handler) StopBackground() {
	if h == nil {
		return
	}
	h.sweepMu.Lock()
	s := h.sweeper
	h.sweeper = nil
	h.sweepMu.Unlock()
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	<-s.doneCh
	slog.Info("task result delivery sweeper stopped")
}

func (s *deliverySweeper) loop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	// One pass immediately: a restart is exactly when reports are owed, and
	// waiting a full interval to find out is the case this exists for.
	s.handler.SweepTaskResultDeliveries(context.Background(), time.Now())
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.handler.SweepTaskResultDeliveries(context.Background(), time.Now())
		}
	}
}

// SweepTaskResultDeliveries retries every report that is due. It is exported so
// a test can drive one pass without a clock.
func (h *Handler) SweepTaskResultDeliveries(ctx context.Context, now time.Time) {
	if h == nil || h.cfg.TaskResultDeliveries == nil {
		return
	}
	due, err := h.cfg.TaskResultDeliveries.ListDueTaskResultDeliveries(ctx, now.Unix(), deliverySweepLimit)
	if err != nil {
		slog.Warn("task result delivery sweep failed", "err", err)
		return
	}
	for _, delivery := range due {
		h.attemptTaskResultDelivery(ctx, delivery.TaskRunID, now)
	}
}
