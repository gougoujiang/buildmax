package handlers

import (
	"net/http"
	"strconv"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/access"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// AuditEventsResponse is a page of a team's audit trail.
type AuditEventsResponse struct {
	Events []model.AuditEvent `json:"events"`
	Total  int                `json:"total"`
}

// listAuditEventsHandler serves GET /api/teams/{team_id}/audit-events.
//
// Owner only. The trail names who did what, including who was refused, which
// is administrative rather than collaborative information — a member does not
// need to see that a colleague was denied something.
func (h *Handler) listAuditEventsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.AuditStore, "audit trail not configured")
	if !ok {
		return
	}
	if _, ok := h.guard().TeamAction(w, r, userID, teamID, access.ActionReadAuditTrail); !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	events, total, err := h.cfg.AuditStore.ListAuditEvents(r.Context(), teamID, limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_audit_events", "team_id", teamID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, AuditEventsResponse{Events: events, Total: total})
}
