package handlers

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/server/handlers/admin"
	artifactroutes "github.com/gougoujiang/buildmax/internal/server/handlers/artifact"
	authroutes "github.com/gougoujiang/buildmax/internal/server/handlers/auth"
	teamroutes "github.com/gougoujiang/buildmax/internal/server/handlers/team"
	"github.com/gougoujiang/buildmax/internal/server/handlers/work"
	"github.com/gougoujiang/buildmax/internal/server/handlers/worker"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
	"github.com/gougoujiang/buildmax/internal/service/task"

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
		Plugins:          h.cfg.PluginService,
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
		JWTSecret:  h.cfg.JWTSecret,
		WorkerLLM:  h.cfg.WorkerLLM,
		TaskRuns:   h.cfg.TaskRunStore,
		Agents:     h.cfg.AgentStore,
		Gateway:    h.cfg.LLMGateway,
		Artifacts:  h.artifactService(),
		Hub:        h.hub,
		OnTerminal: h.terminalListeners,
	})
}

func (h *Handler) terminalListeners(ctx context.Context, info model.TaskRunTerminalInfo) {
	h.reportTaskRunTerminal(ctx, info)
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
		Plugins:          h.cfg.PluginService,
	})
}

// artifactHandler builds the artifact surface.
//
// It is handed the capability rather than the pieces, and holds neither issue,
// task, nor conversation store: an artifact service that could reach them is
// one edit away from an artifact that belongs to a run again.
func (h *Handler) artifactHandler() *artifactroutes.Handler {
	return artifactroutes.New(artifactroutes.Config{
		JWTSecret: h.cfg.JWTSecret,
		Users:     h.cfg.UserStore,
		Teams:     h.cfg.TeamStore,
		Artifacts: h.artifactService(),
		Audit:     h.cfg.Audit,
	})
}

// artifactService returns nil when either half of the capability is missing, so
// "not configured" is decided once here rather than at each route.
func (h *Handler) artifactService() *artifactsvc.Service {
	if h.cfg.ArtifactStore == nil || h.cfg.ArtifactStorage == nil {
		return nil
	}
	return &artifactsvc.Service{
		Artifacts:    h.cfg.ArtifactStore,
		Storage:      h.cfg.ArtifactStorage,
		Audit:        h.cfg.Audit,
		MaxFileBytes: h.cfg.MaxArtifactBytes,
	}
}

// workHandler builds the work surface from the stores those routes read.
func (h *Handler) workHandler() *work.Handler {
	return work.New(work.Config{
		JWTSecret:        h.cfg.JWTSecret,
		Users:            h.cfg.UserStore,
		Issues:           h.cfg.IssueStore,
		IssueComments:    h.cfg.IssueCommentStore,
		Workflows:        h.cfg.WorkflowStore,
		Tasks:            h.cfg.TaskStore,
		TaskRuns:         h.cfg.TaskRunStore,
		Agents:           h.cfg.AgentStore,
		Teams:            h.cfg.TeamStore,
		Conversations:    h.cfg.ConversationStore,
		Messages:         h.cfg.ConversationMessageStore,
		RunOutputs:       h.cfg.RunOutputLister,
		LLMCalls:         h.cfg.LLMCallStore,
		PersistStorage:   h.cfg.PersistStorage,
		RunOutputStorage: h.cfg.RunOutputStorage,
		Artifacts:        h.artifactService(),
		WorkspacesDir:    h.cfg.WorkspacesDir,
		Quota:            h.cfg.QuotaService,
		TitleGenerator:   h.cfg.TitleGenerator,
		ConversationLLM:  h.cfg.ConversationLLMClient,
		Audit:            h.cfg.Audit,
		Hub:              h.hub,
		Turns:            h.turns,
		OnTerminal:       h.terminalListeners,
	})
}

// conversationService answers a Tier 1 turn for the socket, which is the only
// thing the root package still runs itself.
func (h *Handler) conversationService() *conversation.Service {
	var quotaChecker task.QuotaChecker
	if h.cfg.QuotaService != nil {
		quotaChecker = h.cfg.QuotaService
	}
	var workflowSteps task.WorkflowStepLookup
	if h.cfg.WorkflowStore != nil {
		workflowSteps = h.cfg.WorkflowStore
	}
	return &conversation.Service{
		TaskService: &task.Service{
			Agents:         h.cfg.AgentStore,
			Tasks:          h.cfg.TaskStore,
			TaskRuns:       h.cfg.TaskRunStore,
			QuotaChecker:   quotaChecker,
			WorkflowSteps:  workflowSteps,
			TitleGenerator: h.cfg.TitleGenerator,
		},
		ConversationStore: h.cfg.ConversationStore,
		MessageStore:      h.cfg.ConversationMessageStore,
		LLMClient:         h.cfg.ConversationLLMClient,
		TitleGenerator:    h.cfg.TitleGenerator,
		AgentStore:        h.cfg.AgentStore,
	}
}
