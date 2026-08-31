package team

import (
	"net/http"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	teamsvc "github.com/gougoujiang/buildmax/internal/service/team"
)

// sandboxDefaultsResponse is also the PUT request body: a team's default
// sandbox tiers are the whole resource, so GET and PUT share one shape the
// way plugin curation's SetCurationRequest does.
type sandboxDefaultsResponse struct {
	NetworkTier    string `json:"sandbox_network_tier,omitempty"`
	FilesystemTier string `json:"sandbox_filesystem_tier,omitempty"`
}

func (h *Handler) getSandboxDefaultsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	team, err := h.cfg.Teams.GetTeam(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_sandbox_defaults", "user_id", userID, "team_id", teamID)
		return
	}
	if team == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "team not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sandboxDefaultsResponse{
		NetworkTier:    team.DefaultSandboxNetworkTier,
		FilesystemTier: team.DefaultSandboxFilesystemTier,
	})
}

func (h *Handler) setSandboxDefaultsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	var req sandboxDefaultsResponse
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	err := h.teamService().SetSandboxDefaults(r.Context(), teamsvc.SetSandboxDefaultsCmd{
		TeamID:         teamID,
		ActorID:        userID,
		NetworkTier:    req.NetworkTier,
		FilesystemTier: req.FilesystemTier,
	})
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "set_sandbox_defaults", "user_id", userID, "team_id", teamID)
		return
	}
	h.cfg.Audit.UserAction(r.Context(), userID, teamID, coreaudit.TeamSandboxDefaultsSet, "team", teamID,
		req.NetworkTier+"/"+req.FilesystemTier)
	httputil.WriteJSON(w, http.StatusOK, req)
}
