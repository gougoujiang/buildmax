package space

import (
	"net/http"
	"strconv"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	spacesvc "github.com/gougoujiang/buildmax/internal/service/space"
)

type agentInstructionsResponse struct {
	Instructions string `json:"instructions"`
	Revision     int    `json:"revision"`
}

type setAgentInstructionsRequest struct {
	Instructions string `json:"instructions"`
}

func (h *Handler) getAgentInstructionsHandler(w http.ResponseWriter, r *http.Request) {
	userID, spaceID, ok := h.guard().UserAndPathSpace(w, r, h.cfg.Spaces, "spaces not configured")
	if !ok {
		return
	}
	space, err := h.cfg.Spaces.GetSpace(r.Context(), spaceID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_space_agent_instructions", "user_id", userID, "space_id", spaceID)
		return
	}
	if space == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "space not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, agentInstructionsResponse{
		Instructions: space.AgentInstructions,
		Revision:     space.AgentInstructionsRevision,
	})
}

func (h *Handler) setAgentInstructionsHandler(w http.ResponseWriter, r *http.Request) {
	userID, spaceID, ok := h.guard().UserAndPathSpace(w, r, h.cfg.Spaces, "spaces not configured")
	if !ok {
		return
	}
	var req setAgentInstructionsRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	if err := h.spaceService().SetAgentInstructions(r.Context(), spacesvc.SetAgentInstructionsCmd{
		SpaceID: spaceID, ActorID: userID, Instructions: req.Instructions,
	}); err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "set_space_agent_instructions", "user_id", userID, "space_id", spaceID)
		return
	}
	space, err := h.cfg.Spaces.GetSpace(r.Context(), spaceID)
	if err != nil || space == nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "read_saved_space_agent_instructions", "user_id", userID, "space_id", spaceID)
		return
	}
	h.cfg.Audit.UserAction(r.Context(), userID, spaceID, coreaudit.SpaceAgentInstructionsSet, "space", spaceID,
		"revision "+strconv.Itoa(space.AgentInstructionsRevision))
	httputil.WriteJSON(w, http.StatusOK, agentInstructionsResponse{
		Instructions: space.AgentInstructions,
		Revision:     space.AgentInstructionsRevision,
	})
}
