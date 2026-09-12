package space

import (
	"context"
	corespace "github.com/icloudbb/buildmax/internal/core/space"
	"github.com/icloudbb/buildmax/internal/server/handlers/auditexport"
	"net/http"

	coreaudit "github.com/icloudbb/buildmax/internal/core/audit"
)

func (h *Handler) exportAuditEventsHandler(w http.ResponseWriter, r *http.Request) {
	userID, spaceID, ok := h.guard().UserAndPathSpace(w, r, h.cfg.Audits, "audit trail not configured")
	if !ok {
		return
	}
	if _, ok := h.guard().SpaceAction(w, r, userID, spaceID, corespace.ActionReadAuditTrail); !ok {
		return
	}
	store := h.cfg.Audits
	page := func(ctx context.Context, after coreaudit.Cursor, limit int) ([]coreaudit.Event, error) {
		return store.ExportSpaceAuditEvents(ctx, spaceID, after, limit)
	}
	written, truncated := auditexport.Stream(w, r, page, "audit-"+spaceID)
	// Recorded after the stream, so the count is what actually left rather than
	// what was requested. Reading the whole record is itself an action on it,
	// and an export that left no trace would be the one way to consult the
	// trail without appearing in it.
	h.cfg.Audit.UserAction(r.Context(), userID, spaceID, coreaudit.EventsExported,
		"audit_event", "", auditexport.Detail(written, truncated))
}

// exportAdminAuditEventsHandler serves GET /api/admin/audit-events/export.
//
// It takes the same filters as the search it belongs to, including space_id=none
// for the events no space-scoped reader can ever see. The filters are the
// operator's own narrowing of a read they are already permitted, which is why
// they are accepted here and not on the space-scoped route.
