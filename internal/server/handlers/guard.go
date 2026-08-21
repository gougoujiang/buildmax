package handlers

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/server/handlers/admin"
	"github.com/gougoujiang/buildmax/internal/server/handlers/runterminal"
	"github.com/gougoujiang/buildmax/internal/server/handlers/worker"
	"log/slog"
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/access"
)

// guard answers who is calling and whether they may proceed.
//
// Built per call from the stores this handler holds, so it stays correct if a
// store is swapped in a test after construction.
func (h *Handler) guard() *access.Guard {
	return &access.Guard{
		JWTSecret: h.cfg.JWTSecret,
		Users:     h.cfg.UserStore,
		Teams:     h.cfg.TeamStore,
		Grants:    h.cfg.SystemGrantStore,
		Audit:     h.cfg.Audit,
	}
}

// refuseDisabled answers a disabled account and reports whether the caller may
// continue.
//
// The log line names which login path refused, which the guard cannot know.
func (h *Handler) refuseDisabled(w http.ResponseWriter, r *http.Request, user *model.User, handler string) bool {
	if user != nil && user.Disabled() {
		slog.InfoContext(r.Context(), "refused a disabled account",
			"handler", handler, "user_id", user.UserID, "remote", r.RemoteAddr)
	}
	return h.guard().RejectDisabled(w, user)
}

// adminHandler builds the deployment-scoped surface from the fields it needs.
//
// Constructed here rather than injected so internal/server keeps assembling one
// Config; what changed is that administration can only see this slice of it.
func (h *Handler) adminHandler() *admin.Handler {
	return admin.New(admin.Config{
		JWTSecret:        h.cfg.JWTSecret,
		DefaultQuotaTier: h.cfg.DefaultQuotaTier,
		Users:            h.cfg.UserStore,
		LoginCodes:       h.cfg.LoginCodeStore,
		RefreshTokens:    h.cfg.RefreshTokenStore,
		Teams:            h.cfg.TeamStore,
		Grants:           h.cfg.SystemGrantStore,
		Audits:           h.cfg.AuditStore,
		Models:           h.cfg.LLMModelStore,
		Schema:           h.cfg.SchemaStore,
		TaskRuns:         h.cfg.TaskRunStore,
		Quota:            h.cfg.QuotaService,
		Audit:            h.cfg.Audit,
		Deployment:       h.cfg.Deployment,
		DependencyProbes: h.cfg.DependencyProbes,
		RedactedConfig:   h.cfg.RedactedConfig,
	})
}

// workerHandler builds the worker API from the fields it needs.
//
// OnTerminal fans a finished run out to whoever is watching: the connected
// clients this package tracks, and then the server's own callback. The worker
// package is told what to call, not who is listening.
func (h *Handler) workerHandler() *worker.Handler {
	return worker.New(worker.Config{
		JWTSecret:   h.cfg.JWTSecret,
		WorkerToken: h.cfg.WorkerToken,
		WorkerLLM:   h.cfg.WorkerLLM,
		TaskRuns:    h.cfg.TaskRunStore,
		Agents:      h.cfg.AgentStore,
		Gateway:     h.cfg.LLMGateway,
		Hub:         h.hub,
		OnTerminal:  h.terminalListeners,
	})
}

// runAnnouncer closes out a run cancelled from the Portal, reaching the same
// listeners a worker's own report does.
func (h *Handler) runAnnouncer() *runterminal.Announcer {
	return &runterminal.Announcer{Runs: h.cfg.TaskRunStore, Hub: h.hub, On: h.terminalListeners}
}

func (h *Handler) terminalListeners(ctx context.Context, info model.TaskRunTerminalInfo) {
	h.connRegistry.OnTaskRunTerminal(ctx, info)
	if h.cfg.OnTaskRunTerminal != nil {
		h.cfg.OnTaskRunTerminal(ctx, info)
	}
}
