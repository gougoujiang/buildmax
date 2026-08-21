package handlers

import (
	"net/http"
	"strconv"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// AdminAuditEventsResponse is a page of the deployment-wide trail.
type AdminAuditEventsResponse struct {
	Events []model.AuditEvent `json:"events"`
	Total  int                `json:"total"`
}

// listAdminAuditEventsHandler serves GET /api/admin/audit-events.
//
// This is the read the team-scoped route cannot do. A login, a grant, and an
// account action have no team, so ListAuditEvents can never return them —
// which is also why that method stays: a team owner asks a narrower question,
// and handing that reader the wider method is how a team-scoped route quietly
// acquires a deployment-scoped answer.
//
// The response carries what the event already holds and nothing more. There is
// no prompt, no request body, and no resolution of target ids into content:
// searching the trail must not become a way to read across teams.
func (h *Handler) listAdminAuditEventsHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.guard().SystemAdmin(w, r); !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.AuditStore, "audit trail not configured") {
		return
	}
	q := r.URL.Query()
	filter := adminAuditFilter(q)
	limit, offset := httputil.LimitOffset(q, "limit", "offset", httputil.BulkPageDefault, httputil.BulkPageMax)

	events, total, err := h.cfg.AuditStore.SearchAuditEvents(r.Context(), filter, limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_search_audit_events")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, AdminAuditEventsResponse{Events: events, Total: total})
}

// parseUnixParam reads a Unix-seconds query parameter. An unparseable value is
// no bound rather than an error: a filter that rejects the whole request
// because a timestamp was malformed makes an investigation harder than one
// that returns a wider window.
func parseUnixParam(v string) int64 {
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
