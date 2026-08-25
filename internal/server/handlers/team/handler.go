// Package team serves what a team owns: its membership, its agents, its
// webhook keys, its consumption, and its audit trail.
//
// The boundary is the one the product already has. Team is the ownership and
// authorization unit for Portal resources, so a package holding exactly the
// stores a team's own routes read makes that unit something the compiler knows
// about rather than something every reviewer has to remember.
package team

import (
	"net/http"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	"github.com/gougoujiang/buildmax/internal/core/model"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
	"github.com/gougoujiang/buildmax/internal/server/access"
	agentsvc "github.com/gougoujiang/buildmax/internal/service/agent"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/service/plugin"
	"github.com/gougoujiang/buildmax/internal/service/quota"
	teamsvc "github.com/gougoujiang/buildmax/internal/service/team"
	"github.com/gougoujiang/buildmax/internal/service/workflow"
)

type Config struct {
	JWTSecret        string
	DefaultQuotaTier string

	Teams       coreteam.Store
	Users       model.UserStore
	Agents      agentdef.Store
	WebhookKeys model.UserWebhookKeyStore
	Audits      model.AuditStore
	// Workflows answers one question here -- which published workflows still
	// name an agent -- so that deleting one cannot silently break them. Nil
	// leaves that check unmade, which is what a deployment without workflows
	// has.
	Workflows coreworkflow.Store

	Quota *quota.Service
	Audit *audit.Recorder
	// Plugins carries the team half of plugin distribution: which releases a
	// team's background runs may use. Nil in a deployment without a
	// Marketplace, which is why every route here checks before using it.
	Plugins *plugin.Service
}

type Handler struct {
	cfg Config

	teams  *teamsvc.Service
	agents *agentsvc.Service
}

func New(cfg Config) *Handler {
	h := &Handler{cfg: cfg}
	h.teams = newTeamService(cfg)
	h.agents = newTeamAgentService(cfg, newWorkflowUsage(cfg))
	return h
}

func (h *Handler) guard() *access.Guard {
	return &access.Guard{
		JWTSecret: h.cfg.JWTSecret,
		Users:     h.cfg.Users,
		Teams:     h.cfg.Teams,
		Audit:     h.cfg.Audit,
	}
}

// workflowUsage answers only "which published workflows name this agent".
//
// Built from the workflow store alone rather than from a full workflow service:
// the delete guard needs one query, and a service wired for orchestration would
// tie an agent edit to task dispatch.
func newWorkflowUsage(cfg Config) *workflow.Service {
	if cfg.Workflows == nil {
		return nil
	}
	return &workflow.Service{Workflows: cfg.Workflows}
}

func (h *Handler) Register(mux *http.ServeMux) {
	// Teams and members
	mux.HandleFunc("GET /api/teams", h.listTeamsHandler)
	mux.HandleFunc("POST /api/teams", h.createTeamHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/members", h.listTeamMembersHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/members", h.addTeamMemberHandler)
	mux.HandleFunc("DELETE /api/teams/{team_id}/members/{user_id}", h.removeTeamMemberHandler)

	// Agents
	mux.HandleFunc("GET /api/teams/{team_id}/agents", h.listAgentsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/agents", h.createAgentHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/agents/{agent_id}", h.getAgentHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/agents/{agent_id}", h.patchAgentHandler)
	mux.HandleFunc("DELETE /api/teams/{team_id}/agents/{agent_id}", h.deleteAgentHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/agents/{agent_id}/revisions", h.listAgentRevisionsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/agents/{agent_id}/revisions/{revision}/restore", h.restoreAgentRevisionHandler)

	// Webhook keys
	mux.HandleFunc("POST /api/webhook-keys", h.createWebhookKeyHandler)
	mux.HandleFunc("GET /api/webhook-keys", h.listWebhookKeysHandler)
	mux.HandleFunc("DELETE /api/webhook-keys/{key_id}", h.revokeWebhookKeyHandler)

	// Plugin activation
	mux.HandleFunc("GET /api/teams/{team_id}/plugin-activations", h.listPluginActivationsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/plugin-activations", h.activatePluginHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/plugin-activations/{plugin_name}", h.patchPluginActivationHandler)
	mux.HandleFunc("PUT /api/teams/{team_id}/plugin-curation", h.setPluginCurationHandler)

	// Usage and the audit trail
	mux.HandleFunc("GET /api/usage", h.usageHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/usage", h.teamUsageHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/audit-events", h.listAuditEventsHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/audit-events/export", h.exportAuditEventsHandler)
}
