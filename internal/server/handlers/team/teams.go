package team

import (
	"net/http"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	teamsvc "github.com/gougoujiang/buildmax/internal/service/team"
)

type teamResponse struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	PersonalForUserID *string `json:"personal_for_user_id,omitempty"`
	CreatedBy         string  `json:"created_by"`
	CreatedAt         int64   `json:"created_at"`
	UpdatedAt         int64   `json:"updated_at"`
}

type teamMemberResponse struct {
	TeamID    string  `json:"team_id"`
	UserID    string  `json:"user_id"`
	Role      string  `json:"role"`
	CreatedAt int64   `json:"created_at"`
	UserName  *string `json:"user_name,omitempty"`
	UserEmail *string `json:"user_email,omitempty"`
}

type addTeamMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type createTeamRequest struct {
	Name string `json:"name"`
}

func teamToResponse(team model.Team) teamResponse {
	return teamResponse{
		ID:                team.TeamID,
		Name:              team.Name,
		PersonalForUserID: team.PersonalForUserID,
		CreatedBy:         team.CreatedBy,
		CreatedAt:         team.CreatedAt,
		UpdatedAt:         team.UpdatedAt,
	}
}

func teamMemberToResponse(member model.TeamMember, user *model.User) teamMemberResponse {
	resp := teamMemberResponse{
		TeamID:    member.TeamID,
		UserID:    member.UserID,
		Role:      member.Role,
		CreatedAt: member.CreatedAt,
	}
	if user != nil {
		if user.Name != "" {
			resp.UserName = &user.Name
		}
		if user.Email != "" {
			resp.UserEmail = &user.Email
		}
	}
	return resp
}

func (h *Handler) listTeamsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	list, err := h.cfg.Teams.ListTeamsByUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_teams", "user_id", userID)
		return
	}
	out := make([]teamResponse, len(list))
	for i := range list {
		out[i] = teamToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createTeamHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	var req createTeamRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	team, err := h.cfg.Teams.CreateTeam(r.Context(), name, userID, h.cfg.DefaultQuotaTier)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_team", "user_id", userID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, teamToResponse(*team))
}

func (h *Handler) teamService() *teamsvc.Service {
	return &teamsvc.Service{Teams: h.cfg.Teams, Users: h.cfg.Users}
}

func (h *Handler) listTeamMembersHandler(w http.ResponseWriter, r *http.Request) {
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
	members, err := h.teamService().ListMembers(r.Context(), teamID)
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_team_members", "user_id", userID, "team_id", teamID)
		return
	}
	out := make([]teamMemberResponse, len(members))
	for i := range members {
		out[i] = teamMemberToResponse(members[i].Membership, members[i].User)
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) addTeamMemberHandler(w http.ResponseWriter, r *http.Request) {
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
	var req addTeamMemberRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	member, user, err := h.teamService().AddMember(r.Context(), teamsvc.AddMemberCmd{
		TeamID:  teamID,
		ActorID: userID,
		Email:   req.Email,
		Role:    req.Role,
	})
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "add_team_member", "user_id", userID, "team_id", teamID)
		return
	}
	h.cfg.Audit.UserAction(r.Context(), userID, teamID, model.AuditTeamMemberAdded, "user", user.UserID, member.Role)
	httputil.WriteJSON(w, http.StatusCreated, teamMemberToResponse(*member, user))
}

func (h *Handler) removeTeamMemberHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	teamID, ok := httputil.PathValue(w, r, "team_id")
	if !ok {
		return
	}
	targetUserID, ok := httputil.PathValue(w, r, "user_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.guard().ExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}
	err := h.teamService().RemoveMember(r.Context(), teamsvc.RemoveMemberCmd{
		TeamID:       teamID,
		ActorID:      userID,
		TargetUserID: targetUserID,
	})
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "remove_team_member", "team_id", teamID, "member_user_id", targetUserID)
		return
	}
	h.cfg.Audit.UserAction(r.Context(), userID, teamID, model.AuditTeamMemberRemoved, "user", targetUserID, "")
	w.WriteHeader(http.StatusNoContent)
}
