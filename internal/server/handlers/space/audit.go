package space

import (
	corespace "github.com/icloudbb/buildmax/internal/core/space"
	"net/http"
	"strconv"

	coreaudit "github.com/icloudbb/buildmax/internal/core/audit"
	"github.com/icloudbb/buildmax/internal/server/httputil"
)

// AuditEventsResponse is a page of a space's audit trail.
type AuditEventsResponse struct {
	Events []coreaudit.Event `json:"events"`
	Total  int               `json:"total"`
}

// listAuditEventsHandler serves GET /api/spaces/{space_id}/audit-events.
//
// Owner only. The trail names who did what, including who was refused, which
// is administrative rather than collaborative information — a member does not
// need to see that a colleague was denied something.
func (h *Handler) listAuditEventsHandler(w http.ResponseWriter, r *http.Request) {
	userID, spaceID, ok := h.guard().UserAndPathSpace(w, r, h.cfg.Audits, "audit trail not configured")
	if !ok {
		return
	}
	if _, ok := h.guard().SpaceAction(w, r, userID, spaceID, corespace.ActionReadAuditTrail); !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	events, total, err := h.cfg.Audits.ListAuditEvents(r.Context(), spaceID, limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_audit_events", "space_id", spaceID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, AuditEventsResponse{Events: events, Total: total})
}
