package team

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/server/handlers/auditexport"
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/access"
)

func (h *Handler) exportAuditEventsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Audits, "audit trail not configured")
	if !ok {
		return
	}
	if _, ok := h.guard().TeamAction(w, r, userID, teamID, access.ActionReadAuditTrail); !ok {
		return
	}
	store := h.cfg.Audits
	page := func(ctx context.Context, after model.AuditCursor, limit int) ([]model.AuditEvent, error) {
		return store.ExportTeamAuditEvents(ctx, teamID, after, limit)
	}
	written, truncated := auditexport.Stream(w, r, page, "audit-"+teamID)
	// Recorded after the stream, so the count is what actually left rather than
	// what was requested. Reading the whole record is itself an action on it,
	// and an export that left no trace would be the one way to consult the
	// trail without appearing in it.
	h.cfg.Audit.UserAction(r.Context(), userID, teamID, model.AuditEventsExported,
		"audit_event", "", auditexport.Detail(written, truncated))
}

// exportAdminAuditEventsHandler serves GET /api/admin/audit-events/export.
//
// It takes the same filters as the search it belongs to, including team_id=none
// for the events no team-scoped reader can ever see. The filters are the
// operator's own narrowing of a read they are already permitted, which is why
// they are accepted here and not on the team-scoped route.
