package handlers

import (
	"context"
	"log/slog"
)

// StartBackground is retained as the server lifecycle hook. Task completion no
// longer owns a retry loop because results are read directly from TaskRun.
func (h *Handler) StartBackground() {}

func (h *Handler) BeginDrain() {
	if h != nil {
		h.turns.Drain()
	}
}

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

func (h *Handler) WaitTurns(ctx context.Context) {
	if h != nil && !h.turns.Wait(ctx) {
		slog.Warn("conversation turns did not finish before shutdown")
	}
}

func (h *Handler) StopBackground(ctx context.Context) {
	if h != nil {
		h.terminal.Wait(ctx)
	}
}
