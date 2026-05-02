package portalapi

import (
	"errors"
	"net/http"

	"buildmax/internal/infra/db"
	"buildmax/internal/server/httputil"
	"gorm.io/gorm"
)

type AgentResponse struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	TeamID       string `json:"team_id,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	CreatedAt    int64  `json:"created_at"`
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

func agentToResponse(a db.Agent) AgentResponse {
	return AgentResponse{
		ID:           a.AgentID,
		UserID:       a.UserID,
		TeamID:       a.TeamID,
		Name:         a.Name,
		Description:  a.Description,
		Instructions: a.Instructions,
		CreatedAt:    a.CreatedAt,
	}
}

func (h *Handler) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.AgentStore, "agents not configured")
	if !ok {
		return
	}
	list, err := h.cfg.AgentStore.ListAgentsByTeam(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_agents", "user_id", userID, "team_id", teamID)
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
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_agent", "user_id", userID, "team_id", teamID)
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
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_agent", "agent_id", agentID)
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
	agent, err := h.cfg.AgentStore.UpdateAgentInTeam(r.Context(), agentID, teamID, req.Name, req.Description, req.Instructions)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "patch_agent", "agent_id", agentID)
		return
	}
	if agent == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "agent not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, agentToResponse(*agent))
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
	err := h.cfg.AgentStore.DeleteAgentInTeam(r.Context(), agentID, teamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "agent not found")
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "delete_agent", "agent_id", agentID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
