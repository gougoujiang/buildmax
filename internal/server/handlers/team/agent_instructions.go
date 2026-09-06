package team

import (
	"net/http"
	"strconv"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	teamsvc "github.com/gougoujiang/buildmax/internal/service/team"
)

type agentInstructionsResponse struct {
	Instructions string `json:"instructions"`
	Revision     int    `json:"revision"`
}

type setAgentInstructionsRequest struct {
	Instructions string `json:"instructions"`
}

func (h *Handler) getAgentInstructionsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	team, err := h.cfg.Teams.GetTeam(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_team_agent_instructions", "user_id", userID, "team_id", teamID)
		return
	}
	if team == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "team not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, agentInstructionsResponse{
		Instructions: team.AgentInstructions,
		Revision:     team.AgentInstructionsRevision,
	})
}

func (h *Handler) setAgentInstructionsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	var req setAgentInstructionsRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	if err := h.teamService().SetAgentInstructions(r.Context(), teamsvc.SetAgentInstructionsCmd{
		TeamID: teamID, ActorID: userID, Instructions: req.Instructions,
	}); err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "set_team_agent_instructions", "user_id", userID, "team_id", teamID)
		return
	}
	team, err := h.cfg.Teams.GetTeam(r.Context(), teamID)
	if err != nil || team == nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "read_saved_team_agent_instructions", "user_id", userID, "team_id", teamID)
		return
	}
	h.cfg.Audit.UserAction(r.Context(), userID, teamID, coreaudit.TeamAgentInstructionsSet, "team", teamID,
		"revision "+strconv.Itoa(team.AgentInstructionsRevision))
	httputil.WriteJSON(w, http.StatusOK, agentInstructionsResponse{
		Instructions: team.AgentInstructions,
		Revision:     team.AgentInstructionsRevision,
	})
}
