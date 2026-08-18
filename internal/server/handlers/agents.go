package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"gorm.io/gorm"
)

type AgentResponse struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	TeamID       string `json:"team_id,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	Revision     int    `json:"revision"`
	CreatedAt    int64  `json:"created_at"`
}

type agentRevisionResponse struct {
	ID           string `json:"id"`
	AgentID      string `json:"agent_id"`
	Revision     int    `json:"revision"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	CreatedBy    string `json:"created_by"`
	CreatedAt    int64  `json:"created_at"`
}

type agentRevisionListResponse struct {
	Revisions []agentRevisionResponse `json:"revisions"`
	Total     int                     `json:"total"`
}

type createAgentRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
}

type patchAgentRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
}

func agentToResponse(a model.Agent) AgentResponse {
	return AgentResponse{
		ID:           a.AgentID,
		UserID:       a.UserID,
		TeamID:       a.TeamID,
		Name:         a.Name,
		Description:  a.Description,
		Instructions: a.Instructions,
		Revision:     a.Revision,
		CreatedAt:    a.CreatedAt,
	}
}

// workflowNameList renders the workflows blocking a delete as "name (id)",
// because a name alone is ambiguous and an ID alone means nothing to a reader.
func workflowNameList(workflows []model.Workflow) string {
	parts := make([]string, len(workflows))
	for i := range workflows {
		parts[i] = workflows[i].Name + " (" + workflows[i].WorkflowID + ")"
	}
	return strings.Join(parts, ", ")
}

func agentRevisionToResponse(rev model.AgentRevision) agentRevisionResponse {
	return agentRevisionResponse{
		ID:           rev.AgentRevisionID,
		AgentID:      rev.AgentID,
		Revision:     rev.Revision,
		Name:         rev.Name,
		Description:  rev.Description,
		Instructions: rev.Instructions,
		CreatedBy:    rev.CreatedBy,
		CreatedAt:    rev.CreatedAt,
	}
}

func (h *Handler) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.AgentStore, "agents not configured")
	if !ok {
		return
	}
	list, err := h.cfg.AgentStore.ListAgentsByTeam(r.Context(), teamID)
	if err != nil {
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
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.AgentStore, "agents not configured")
	if !ok {
		return
	}
	if _, ok := h.authorizeTeamAction(w, r, userID, teamID, actionManageAgents); !ok {
		return
	}
	var req createAgentRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "name required")
		return
	}
	agent, err := h.cfg.AgentStore.CreateAgentInTeam(r.Context(), teamID, userID, req.Name, req.Description, req.Instructions)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_agent", "user_id", userID, "team_id", teamID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, agentToResponse(*agent))
}

func (h *Handler) getAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.AgentStore, "agents not configured")
	if !ok {
		return
	}
	agentID, ok := pathValueRequired(w, r, "agent_id")
	if !ok {
		return
	}
	agent, err := h.cfg.AgentStore.GetAgent(r.Context(), agentID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_agent", "agent_id", agentID)
		return
	}
	if agent == nil || agent.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "agent not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, agentToResponse(*agent))
}

func (h *Handler) patchAgentHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.AgentStore, "agents not configured")
	if !ok {
		return
	}
	if _, ok := h.authorizeTeamAction(w, r, userID, teamID, actionManageAgents); !ok {
		return
	}
	agentID, ok := pathValueRequired(w, r, "agent_id")
	if !ok {
		return
	}
	var req patchAgentRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "name required")
		return
	}
	agent, err := h.cfg.AgentStore.UpdateAgentInTeam(r.Context(), agentID, teamID, userID, req.Name, req.Description, req.Instructions)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "patch_agent", "agent_id", agentID)
		return
	}
	if agent == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "agent not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, agentToResponse(*agent))
}

// listAgentRevisionsHandler returns an agent's recorded definitions, newest
// first. Reading history needs no more than team membership: it is the same
// content the agent itself exposes, at earlier points in time.
func (h *Handler) listAgentRevisionsHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.AgentStore, "agents not configured")
	if !ok {
		return
	}
	agentID, ok := pathValueRequired(w, r, "agent_id")
	if !ok {
		return
	}
	agent, err := h.cfg.AgentStore.GetAgent(r.Context(), agentID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_agent_revisions", "agent_id", agentID)
		return
	}
	if agent == nil || agent.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "agent not found")
		return
	}
	limit, offset := parseLimitOffset(r.URL.Query(), "limit", "offset", 20, 100)
	list, total, err := h.cfg.AgentStore.ListAgentRevisions(r.Context(), agentID, limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_agent_revisions", "agent_id", agentID)
		return
	}
	out := make([]agentRevisionResponse, len(list))
	for i := range list {
		out[i] = agentRevisionToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, agentRevisionListResponse{Revisions: out, Total: total})
}

// restoreAgentRevisionHandler writes an earlier revision's content back to the
// agent. That is an ordinary edit — it appends a new revision rather than
// rewinding to the old one — so it needs the permission an edit needs.
func (h *Handler) restoreAgentRevisionHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.AgentStore, "agents not configured")
	if !ok {
		return
	}
	if _, ok := h.authorizeTeamAction(w, r, userID, teamID, actionManageAgents); !ok {
		return
	}
	agentID, ok := pathValueRequired(w, r, "agent_id")
	if !ok {
		return
	}
	revision, ok := pathValueInt(w, r, "revision")
	if !ok {
		return
	}
	agent, err := h.cfg.AgentStore.GetAgent(r.Context(), agentID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "restore_agent_revision", "agent_id", agentID)
		return
	}
	if agent == nil || agent.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "agent not found")
		return
	}
	rev, err := h.cfg.AgentStore.GetAgentRevision(r.Context(), agentID, revision)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "restore_agent_revision", "agent_id", agentID)
		return
	}
	if rev == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "agent revision not found")
		return
	}
	updated, err := h.cfg.AgentStore.UpdateAgentInTeam(r.Context(), agentID, teamID, userID, rev.Name, rev.Description, rev.Instructions)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "restore_agent_revision", "agent_id", agentID)
		return
	}
	if updated == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "agent not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, agentToResponse(*updated))
}

func (h *Handler) deleteAgentHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.AgentStore, "agents not configured")
	if !ok {
		return
	}
	if _, ok := h.authorizeTeamAction(w, r, userID, teamID, actionManageAgents); !ok {
		return
	}
	agentID, ok := pathValueRequired(w, r, "agent_id")
	if !ok {
		return
	}
	// Deleting an agent a published workflow still names would leave that
	// workflow unable to run, and the operator would only find out at the next
	// run. Name the workflows instead and let them be fixed or archived first.
	if h.cfg.WorkflowStore != nil {
		using, err := h.workflowService().PublishedWorkflowsUsingAgent(r.Context(), teamID, agentID)
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "delete_agent", "agent_id", agentID)
			return
		}
		if len(using) > 0 {
			httputil.WriteJSONError(w, http.StatusConflict,
				"agent is used by published workflows: "+workflowNameList(using))
			return
		}
	}
	err := h.cfg.AgentStore.DeleteAgentInTeam(r.Context(), agentID, teamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "agent not found")
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "delete_agent", "agent_id", agentID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
