package handlers

import (
	"net/http"
	"strings"

	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// AdminTeam is one team as an administrator sees it: metadata only.
//
// There is deliberately no field here that a team member wrote or an agent
// produced. The rule the design states — if a member wrote it or an agent
// produced it, it is content — is what keeps this struct from growing an
// "issue count" that turns into an issue list that turns into issue titles.
type AdminTeam struct {
	TeamID string `json:"team_id"`
	Name   string `json:"name"`
	// Personal marks a user's own space rather than a collaborative team.
	Personal    bool   `json:"personal"`
	QuotaTier   string `json:"quota_tier,omitempty"`
	MemberCount int    `json:"member_count"`
	CreatedBy   string `json:"created_by,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

// AdminTeamsResponse is a page of teams.
type AdminTeamsResponse struct {
	Teams []AdminTeam `json:"teams"`
	Total int         `json:"total"`
}

// AdminTeamDetail adds the membership and the capacity the team is using.
type AdminTeamDetail struct {
	AdminTeam
	Members []AdminTeamMember `json:"members"`
	// Usage is nil when the deployment reports no quota, so a reader can tell
	// "no limits configured" from "using nothing".
	Usage *usageResponse `json:"usage,omitempty"`
}

// AdminTeamMember names one member and their role.
type AdminTeamMember struct {
	UserID string `json:"user_id"`
	Email  string `json:"email,omitempty"`
	Role   string `json:"role"`
}

// listAdminTeamsHandler serves GET /api/admin/teams.
func (h *Handler) listAdminTeamsHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireSystemAdmin(w, r); !ok {
		return
	}
	if !h.requireStore(w, h.cfg.TeamStore, "teams not configured") {
		return
	}
	limit, offset := parseLimitOffset(r.URL.Query(), "limit", "offset", bulkPageDefault, bulkPageMax)
	teams, total, err := h.cfg.TeamStore.ListAllTeams(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_list_teams")
		return
	}
	ids := make([]string, 0, len(teams))
	for _, team := range teams {
		ids = append(ids, team.TeamID)
	}
	counts, err := h.cfg.TeamStore.CountTeamMembers(r.Context(), ids)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_list_teams", "counts")
		return
	}

	out := make([]AdminTeam, 0, len(teams))
	for _, team := range teams {
		out = append(out, AdminTeam{
			TeamID:      team.TeamID,
			Name:        team.Name,
			Personal:    team.PersonalForUserID != nil,
			QuotaTier:   team.QuotaTier,
			MemberCount: counts[team.TeamID],
			CreatedBy:   team.CreatedBy,
			CreatedAt:   team.CreatedAt,
		})
	}
	httputil.WriteJSON(w, http.StatusOK, AdminTeamsResponse{Teams: out, Total: total})
}

// getAdminTeamHandler serves GET /api/admin/teams/{team_id}.
func (h *Handler) getAdminTeamHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireSystemAdmin(w, r); !ok {
		return
	}
	if !h.requireStore(w, h.cfg.TeamStore, "teams not configured") {
		return
	}
	teamID, ok := pathValueRequired(w, r, "team_id")
	if !ok {
		return
	}
	team, err := h.cfg.TeamStore.GetTeam(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_get_team", "team_id", teamID)
		return
	}
	if team == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "team not found")
		return
	}
	members, err := h.cfg.TeamStore.ListTeamMembers(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_get_team", "members")
		return
	}

	detail := AdminTeamDetail{
		AdminTeam: AdminTeam{
			TeamID:      team.TeamID,
			Name:        team.Name,
			Personal:    team.PersonalForUserID != nil,
			QuotaTier:   team.QuotaTier,
			MemberCount: len(members),
			CreatedBy:   team.CreatedBy,
			CreatedAt:   team.CreatedAt,
		},
		Members: make([]AdminTeamMember, 0, len(members)),
	}
	for _, member := range members {
		row := AdminTeamMember{UserID: member.UserID, Role: member.Role}
		if h.cfg.UserStore != nil {
			// A membership naming an account that is gone is not expected.
			// Showing the user id beats refusing to describe the team.
			if user, err := h.cfg.UserStore.GetUser(r.Context(), member.UserID); err == nil && user != nil {
				row.Email = user.Email
			}
		}
		detail.Members = append(detail.Members, row)
	}

	// Usage is what makes this page answer an operator's question rather than
	// just listing rows. It is a count of runs and tokens — capacity, not
	// content.
	if h.cfg.QuotaService != nil {
		if info, err := h.cfg.QuotaService.GetUsage(r.Context(), teamID); err == nil {
			detail.Usage = &usageResponse{
				RunCount:           info.RunCount,
				TotalTokens:        info.TotalTokens,
				TierName:           info.TierName,
				PeriodDays:         info.PeriodDays,
				MaxRunsPerPeriod:   info.MaxRunsPerPeriod,
				MaxTokensPerPeriod: info.MaxTokensPerPeriod,
			}
		}
	}
	httputil.WriteJSON(w, http.StatusOK, detail)
}
