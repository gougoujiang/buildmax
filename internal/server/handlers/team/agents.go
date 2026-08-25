package team

import (
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"net/http"
	"time"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/service/agent"
	"github.com/gougoujiang/buildmax/internal/service/workflow"
)

type AgentResponse struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	TeamID       string    `json:"team_id,omitempty"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Instructions string    `json:"instructions"`
	Plugins      []string  `json:"plugins,omitempty"`
	Revision     int       `json:"revision"`
	CreatedAt    time.Time `json:"created_at"`
}

type agentRevisionResponse struct {
	AgentID      string    `json:"agent_id"`
	Revision     int       `json:"revision"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Instructions string    `json:"instructions"`
	Plugins      []string  `json:"plugins,omitempty"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}

type agentRevisionListResponse struct {
	Revisions []agentRevisionResponse `json:"revisions"`
	Total     int                     `json:"total"`
}

type createAgentRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	// Plugins names catalog plugins, never releases: the version comes from
	// the team's activation.
	Plugins []string `json:"plugins,omitempty"`
}

// patchAgentRequest replaces the whole definition, plugins included. An absent
// list is an empty one, because a patch that could not clear the selection
// would leave no way to stop an agent loading a plugin.
type patchAgentRequest struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	Plugins      []string `json:"plugins,omitempty"`
}

func agentToResponse(a agentdef.Agent) AgentResponse {
	return AgentResponse{
		ID:           a.ID,
		UserID:       a.UserID,
		TeamID:       a.TeamID,
		Name:         a.Name,
		Description:  a.Description,
		Instructions: a.Instructions,
		Plugins:      a.Plugins,
		Revision:     a.Revision,
		CreatedAt:    a.CreatedAt,
	}
}

func agentRevisionToResponse(rev agentdef.Revision) agentRevisionResponse {
	return agentRevisionResponse{
		AgentID:      rev.AgentID,
		Revision:     rev.Revision,
		Name:         rev.Name,
		Description:  rev.Description,
		Instructions: rev.Instructions,
		Plugins:      rev.Plugins,
		CreatedBy:    rev.CreatedBy,
		CreatedAt:    rev.CreatedAt,
	}
}

func (h *Handler) agentService() *agent.Service {
	return h.agents
}

func newTeamAgentService(cfg Config, workflowUsage *workflow.Service) *agent.Service {
	svc := &agent.Service{Agents: cfg.Agents}
	if workflowUsage != nil {
		svc.Workflows = workflowUsage
	}
	// Nil when the deployment has no Marketplace, which is what makes naming a
	// plugin a refusal there rather than a stored selection nothing resolves.
	if cfg.Plugins != nil && cfg.Plugins.Activations != nil {
		svc.Plugins = cfg.Plugins
	}
	return svc
}

// writeAgentServiceError maps what an agent write can refuse. It routes through
// the activation mapping because an edit that names a plugin can fail for that
// plugin's reasons — the release contributes a hook, the team has not activated
// it — and those answers belong to the caller, not in a 500.
func (h *Handler) writeAgentServiceError(w http.ResponseWriter, err error) bool {
	return writePluginActivationError(w, err)
}

func (h *Handler) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Agents, "agents not configured")
	if !ok {
		return
	}
	list, err := h.agentService().ListAgents(r.Context(), teamID)
	if err != nil {
		if h.writeAgentServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_agents", "user_id", userID, "team_id", teamID)
		return
	}
	out := make([]AgentResponse, len(list))
	for i := range list {
		out[i] = agentToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createAgentHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Agents, "agents not configured")
	if !ok {
		return
	}
	if _, ok := h.guard().TeamAction(w, r, userID, teamID, coreteam.ActionManageAgents); !ok {
		return
	}
	var req createAgentRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	created, err := h.agentService().CreateAgent(r.Context(), agent.CreateCmd{
		TeamID:       teamID,
		UserID:       userID,
		Name:         req.Name,
		Description:  req.Description,
		Instructions: req.Instructions,
		Plugins:      req.Plugins,
	})
	if err != nil {
		if h.writeAgentServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_agent", "user_id", userID, "team_id", teamID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, agentToResponse(*created))
}

func (h *Handler) getAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Agents, "agents not configured")
	if !ok {
		return
	}
	agentID, ok := httputil.PathValue(w, r, "agent_id")
	if !ok {
		return
	}
	found, err := h.agentService().GetAgent(r.Context(), teamID, agentID)
	if err != nil {
		if h.writeAgentServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_agent", "agent_id", agentID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, agentToResponse(*found))
}

func (h *Handler) patchAgentHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Agents, "agents not configured")
	if !ok {
		return
	}
	if _, ok := h.guard().TeamAction(w, r, userID, teamID, coreteam.ActionManageAgents); !ok {
		return
	}
	agentID, ok := httputil.PathValue(w, r, "agent_id")
	if !ok {
		return
	}
	var req patchAgentRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	updated, err := h.agentService().UpdateAgent(r.Context(), agent.UpdateCmd{
		TeamID:       teamID,
		UserID:       userID,
		AgentID:      agentID,
		Name:         req.Name,
		Description:  req.Description,
		Instructions: req.Instructions,
		Plugins:      req.Plugins,
	})
	if err != nil {
		if h.writeAgentServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "patch_agent", "agent_id", agentID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, agentToResponse(*updated))
}

// listAgentRevisionsHandler returns an agent's recorded definitions, newest
// first. Reading history needs no more than team membership: it is the same
// content the agent itself exposes, at earlier points in time.
func (h *Handler) listAgentRevisionsHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Agents, "agents not configured")
	if !ok {
		return
	}
	agentID, ok := httputil.PathValue(w, r, "agent_id")
	if !ok {
		return
	}
	limit, offset := httputil.LimitOffset(r.URL.Query(), "limit", "offset", httputil.BrowsePageDefault, httputil.BrowsePageMax)
	list, total, err := h.agentService().ListRevisions(r.Context(), teamID, agentID, limit, offset)
	if err != nil {
		if h.writeAgentServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_agent_revisions", "agent_id", agentID)
		return
	}
	out := make([]agentRevisionResponse, len(list))
	for i := range list {
		out[i] = agentRevisionToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, agentRevisionListResponse{Revisions: out, Total: total})
}

func (h *Handler) restoreAgentRevisionHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Agents, "agents not configured")
	if !ok {
		return
	}
	if _, ok := h.guard().TeamAction(w, r, userID, teamID, coreteam.ActionManageAgents); !ok {
		return
	}
	agentID, ok := httputil.PathValue(w, r, "agent_id")
	if !ok {
		return
	}
	revision, ok := httputil.PathValueInt(w, r, "revision")
	if !ok {
		return
	}
	updated, err := h.agentService().RestoreRevision(r.Context(), agent.RestoreRevisionCmd{
		TeamID:   teamID,
		UserID:   userID,
		AgentID:  agentID,
		Revision: revision,
	})
	if err != nil {
		if h.writeAgentServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "restore_agent_revision", "agent_id", agentID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, agentToResponse(*updated))
}

func (h *Handler) deleteAgentHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Agents, "agents not configured")
	if !ok {
		return
	}
	if _, ok := h.guard().TeamAction(w, r, userID, teamID, coreteam.ActionManageAgents); !ok {
		return
	}
	agentID, ok := httputil.PathValue(w, r, "agent_id")
	if !ok {
		return
	}
	if err := h.agentService().DeleteAgent(r.Context(), teamID, agentID); err != nil {
		if h.writeAgentServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "delete_agent", "agent_id", agentID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
