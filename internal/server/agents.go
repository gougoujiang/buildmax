package server

import (
	"encoding/json"
	"net/http"

	"buildmax/internal/model"
)

// AgentResponse is one agent in the list/get response (snake_case).
type AgentResponse struct {
	ID           string `json:"id"`
	WorkspaceID  string `json:"workspace_id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	CreatedAt    int64  `json:"created_at"`
}

// createAgentRequest is the JSON body for POST /api/workspaces/{workspace_id}/agents.
type createAgentRequest struct {
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

// listAgentsHandler handles GET /api/workspaces/{workspace_id}/agents.
func (s *Server) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.AgentStore, "agents not configured") {
		return
	}
	list, err := s.cfg.AgentStore.ListAgentsByWorkspace(r.Context(), workspaceID)
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

// createAgentHandler handles POST /api/workspaces/{workspace_id}/agents.
func (s *Server) createAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.AgentStore, "agents not configured") {
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
	agent, err := s.cfg.AgentStore.CreateAgent(r.Context(), workspaceID, req.Name, req.Description, req.Instructions)
	if err != nil {
		writeInternalError(w, err, "handler", "create_agent", "workspace_id", workspaceID)
		return
	}
	writeJSON(w, http.StatusCreated, agentToResponse(*agent))
}

// getAgentHandler handles GET /api/workspaces/{workspace_id}/agents/{agent_id}.
func (s *Server) getAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.AgentStore, "agents not configured") {
		return
	}
	agentID := r.PathValue("agent_id")
	if agentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id required")
		return
	}
	agent, err := s.cfg.AgentStore.GetAgent(r.Context(), agentID)
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
