package portal

import (
	"encoding/json"
	"errors"
	"net/http"

	"buildmax/internal/model"
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

func agentToResponse(a model.Agent) AgentResponse {
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
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.AgentStore, "agents not configured") {
		return
	}
	list, err := h.cfg.AgentStore.ListAgentsByWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeInternalError(w, err, "handler", "list_agents", "workspace_id", workspaceID)
		return
	}
	out := make([]AgentResponse, len(list))
	for i := range list {
		out[i] = agentToResponse(list[i])
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) createAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.AgentStore, "agents not configured") {
		return
	}
	var req createAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name required")
		return
	}
	agent, err := h.cfg.AgentStore.CreateAgent(r.Context(), workspaceID, req.Name, req.Description, req.Instructions)
	if err != nil {
		writeInternalError(w, err, "handler", "create_agent", "workspace_id", workspaceID)
		return
	}
	writeJSON(w, http.StatusCreated, agentToResponse(*agent))
}

func (h *Handler) getAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.AgentStore, "agents not configured") {
		return
	}
	agentID := r.PathValue("agent_id")
	if agentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id required")
		return
	}
	agent, err := h.cfg.AgentStore.GetAgent(r.Context(), agentID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_agent", "agent_id", agentID)
		return
	}
	if agent == nil || agent.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusNotFound, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, agentToResponse(*agent))
}

func (h *Handler) patchAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.AgentStore, "agents not configured") {
		return
	}
	agentID := r.PathValue("agent_id")
	if agentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id required")
		return
	}
	var req patchAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name required")
		return
	}
	agent, err := h.cfg.AgentStore.UpdateAgent(r.Context(), agentID, workspaceID, req.Name, req.Description, req.Instructions)
	if err != nil {
		writeInternalError(w, err, "handler", "patch_agent", "agent_id", agentID)
		return
	}
	if agent == nil {
		writeJSONError(w, http.StatusNotFound, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, agentToResponse(*agent))
}

func (h *Handler) deleteAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.AgentStore, "agents not configured") {
		return
	}
	agentID := r.PathValue("agent_id")
	if agentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id required")
		return
	}
	err := h.cfg.AgentStore.DeleteAgent(r.Context(), agentID, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSONError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeInternalError(w, err, "handler", "delete_agent", "agent_id", agentID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
