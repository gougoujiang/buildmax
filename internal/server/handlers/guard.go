package handlers

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/server/handlers/admin"
	authroutes "github.com/gougoujiang/buildmax/internal/server/handlers/auth"
	"github.com/gougoujiang/buildmax/internal/server/handlers/runterminal"
	teamroutes "github.com/gougoujiang/buildmax/internal/server/handlers/team"
	"github.com/gougoujiang/buildmax/internal/server/handlers/worker"

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

// authHandler builds the session routes from the fields they need.
func (h *Handler) authHandler() *authroutes.Handler {
	return authroutes.New(authroutes.Config{
		JWTSecret:            h.cfg.JWTSecret,
		AllowSignup:          h.cfg.AllowSignup,
		DefaultQuotaTier:     h.cfg.DefaultQuotaTier,
		AccessTokenTTL:       h.cfg.AccessTokenTTL,
		RefreshTokenTTL:      h.cfg.RefreshTokenTTL,
		RefreshRotationGrace: h.cfg.RefreshRotationGrace,
		Users:                h.cfg.UserStore,
		LoginCodes:           h.cfg.LoginCodeStore,
		Passwords:            h.cfg.PasswordStore,
		RefreshTokens:        h.cfg.RefreshTokenStore,
		Audit:                h.cfg.Audit,
	})
}

// teamHandler builds the team surface from the stores a team's own routes read.
func (h *Handler) teamHandler() *teamroutes.Handler {
	return teamroutes.New(teamroutes.Config{
		JWTSecret:        h.cfg.JWTSecret,
		DefaultQuotaTier: h.cfg.DefaultQuotaTier,
		Teams:            h.cfg.TeamStore,
		Users:            h.cfg.UserStore,
		Agents:           h.cfg.AgentStore,
		WebhookKeys:      h.cfg.UserWebhookKeyStore,
		Audits:           h.cfg.AuditStore,
		Workflows:        h.cfg.WorkflowStore,
		Quota:            h.cfg.QuotaService,
		Audit:            h.cfg.Audit,
	})
}
