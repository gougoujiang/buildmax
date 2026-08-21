package team

import (
	"net/http"

	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

type usageResponse struct {
	RunCount           int    `json:"run_count"`
	TotalTokens        int    `json:"total_tokens"`
	TierName           string `json:"tier"`
	PeriodDays         int    `json:"period_days"`
	MaxRunsPerPeriod   *int   `json:"max_runs_per_period,omitempty"`
	MaxTokensPerPeriod *int   `json:"max_tokens_per_period,omitempty"`
}

// usageHandler keeps the legacy /api/usage route as a personal-team alias.
func (h *Handler) usageHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	if h.cfg.Teams == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "teams not configured")
		return
	}
	team, err := h.cfg.Teams.GetPersonalTeamByUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "usage_get_personal_team", "user_id", userID)
		return
	}
	if team == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "personal team not found")
		return
	}
	h.writeUsageForTeam(w, r, team.TeamID)
}

func (h *Handler) teamUsageHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	teamID, ok := httputil.PathValue(w, r, "team_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.guard().ExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}
	h.writeUsageForTeam(w, r, teamID)
}

func (h *Handler) writeUsageForTeam(w http.ResponseWriter, r *http.Request, teamID string) {
	if h.cfg.Quota == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "usage not available")
		return
	}
	info, err := h.cfg.Quota.GetUsage(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "usage", "team_id", teamID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, usageResponse{
		RunCount:           info.RunCount,
		TotalTokens:        info.TotalTokens,
		TierName:           info.TierName,
		PeriodDays:         info.PeriodDays,
		MaxRunsPerPeriod:   info.MaxRunsPerPeriod,
		MaxTokensPerPeriod: info.MaxTokensPerPeriod,
	})
}
