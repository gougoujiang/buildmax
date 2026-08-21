package admin

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/server/handlers/auditexport"
	"net/http"

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
	if !httputil.RequireStore(w, h.cfg.Audits, "audit trail not configured") {
		return
	}
	q := r.URL.Query()
	filter := auditexport.AdminFilter(q)
	limit, offset := httputil.LimitOffset(q, "limit", "offset", httputil.BulkPageDefault, httputil.BulkPageMax)

	events, total, err := h.cfg.Audits.SearchAuditEvents(r.Context(), filter, limit, offset)
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

func (h *Handler) exportAdminAuditEventsHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := h.guard().SystemAdmin(w, r)
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.Audits, "audit trail not configured") {
		return
	}
	filter := auditexport.AdminFilter(r.URL.Query())
	store := h.cfg.Audits
	page := func(ctx context.Context, after model.AuditCursor, limit int) ([]model.AuditEvent, error) {
		return store.ExportAuditEvents(ctx, filter, after, limit)
	}
	written, truncated := auditexport.Stream(w, r, page, "audit-deployment")
	// An export narrowed to one team is recorded in that team's trail as well,
	// so a team owner can see that the deployment read their record. An
	// unfiltered one has no team to name and stays deployment-scoped.
	h.cfg.Audit.Record(r.Context(), model.AuditEvent{
		TeamID:     filter.TeamID,
		ActorType:  model.AuditActorUser,
		ActorID:    adminID,
		Action:     model.AuditEventsExported,
		TargetType: "audit_event",
		Detail:     auditexport.Detail(written, truncated),
	})
}

// streamAuditExport writes the trail out in the requested format and returns
// how many events it wrote and whether it stopped at the cap.
//
// The response streams rather than buffering: an export is bounded by the size
// of the trail, not by a page size, and holding a deployment's whole history in
// memory to serve it would make the cap a memory limit instead of a policy one.
//
// A store error partway through cannot become an HTTP status — the header is
// long gone by then. It stops the stream and is logged, and the audit event
// records the short count, so the file's own record says it is incomplete.
