package portal

import (
	"errors"
	"net/http"

	"buildmax/internal/server/httputil"
	"buildmax/internal/storage/entity"
	"gorm.io/gorm"
)

type AgentResponse struct {
	ID           string `json:"id"`
	WorkspaceID  string `json:"workspace_id"`
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

func agentToResponse(a entity.Agent) AgentResponse {
	return AgentResponse{
		ID:           a.AgentID,
		WorkspaceID:  a.WorkspaceID,
		Name:         a.Name,
		Description:  a.Description,
		Instructions: a.Instructions,
		CreatedAt:    a.CreatedAt,
	}
}

func (h *Handler) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAndStore(w, r, "workspace_id", h.cfg.AgentStore, "agents not configured")
	if !ok {
		return
	}
	list, err := h.cfg.AgentStore.ListAgentsByWorkspace(r.Context(), workspaceID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_agents", "workspace_id", workspaceID)
		return
	}
	out := make([]AgentResponse, len(list))
	for i := range list {
		out[i] = agentToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAndStore(w, r, "workspace_id", h.cfg.AgentStore, "agents not configured")
	if !ok {
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
	agent, err := h.cfg.AgentStore.CreateAgent(r.Context(), workspaceID, req.Name, req.Description, req.Instructions)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_agent", "workspace_id", workspaceID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, agentToResponse(*agent))
}

func (h *Handler) getAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAndStore(w, r, "workspace_id", h.cfg.AgentStore, "agents not configured")
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
	if agent == nil || agent.WorkspaceID != workspaceID {
		httputil.WriteJSONError(w, http.StatusNotFound, "agent not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, agentToResponse(*agent))
}

func (h *Handler) patchAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAndStore(w, r, "workspace_id", h.cfg.AgentStore, "agents not configured")
	if !ok {
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
	agent, err := h.cfg.AgentStore.UpdateAgent(r.Context(), agentID, workspaceID, req.Name, req.Description, req.Instructions)
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
	_, workspaceID, ok := h.withWorkspaceAndStore(w, r, "workspace_id", h.cfg.AgentStore, "agents not configured")
	if !ok {
		return
	}
	agentID, ok := pathValueRequired(w, r, "agent_id")
	if !ok {
		return
	}
	err := h.cfg.AgentStore.DeleteAgent(r.Context(), agentID, workspaceID)
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
