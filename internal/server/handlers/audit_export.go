package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// auditExportPage is how many events one round trip to the store fetches. The
// export walks the trail with a keyset cursor, so this bounds memory without
// bounding the answer.
const auditExportPage = 500

// auditExportMax bounds one export.
//
// It is set far above any trail a private deployment is expected to accumulate,
// and it exists so a single request cannot stream without end. Reaching it is
// recorded in the audit event the export writes, because an export that stopped
// early and did not say so would be read as a complete record — which is the
// one thing an evidence export must never be.
const auditExportMax = 200_000

// auditExportHeader is the CSV header, and also fixes the field order both
// formats use.
var auditExportHeader = []string{
	"audit_event_id", "created_at", "team_id", "actor_type", "actor_id",
	"action", "target_type", "target_id", "detail",
}

// auditPageFunc fetches one page of events after a cursor. It is what separates
// the team-scoped export from the deployment-scoped one; everything below this
// line is identical for both.
type auditPageFunc func(ctx context.Context, after model.AuditCursor, limit int) ([]model.AuditEvent, error)

// exportAuditEventsHandler serves
// GET /api/teams/{team_id}/audit-events/export.
//
// Owner only, the same reader as the trail itself: an export is the trail, in a
// file. It carries the whole of a team's trail rather than a filtered slice —
// the reason to export is to keep or examine the record elsewhere, and a filter
// applied on the way out is a decision the file cannot show it made.
func (h *Handler) exportAuditEventsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.AuditStore, "audit trail not configured")
	if !ok {
		return
	}
	if _, ok := h.authorizeTeamAction(w, r, userID, teamID, actionReadAuditTrail); !ok {
		return
	}
	store := h.cfg.AuditStore
	page := func(ctx context.Context, after model.AuditCursor, limit int) ([]model.AuditEvent, error) {
		return store.ExportTeamAuditEvents(ctx, teamID, after, limit)
	}
	written, truncated := h.streamAuditExport(w, r, page, "audit-"+teamID)
	// Recorded after the stream, so the count is what actually left rather than
	// what was requested. Reading the whole record is itself an action on it,
	// and an export that left no trace would be the one way to consult the
	// trail without appearing in it.
	h.cfg.Audit.UserAction(r.Context(), userID, teamID, model.AuditEventsExported,
		"audit_event", "", auditExportDetail(written, truncated))
}

// exportAdminAuditEventsHandler serves GET /api/admin/audit-events/export.
//
// It takes the same filters as the search it belongs to, including team_id=none
// for the events no team-scoped reader can ever see. The filters are the
// operator's own narrowing of a read they are already permitted, which is why
// they are accepted here and not on the team-scoped route.
func (h *Handler) exportAdminAuditEventsHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := h.requireSystemAdmin(w, r)
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.AuditStore, "audit trail not configured") {
		return
	}
	filter := adminAuditFilter(r.URL.Query())
	store := h.cfg.AuditStore
	page := func(ctx context.Context, after model.AuditCursor, limit int) ([]model.AuditEvent, error) {
		return store.ExportAuditEvents(ctx, filter, after, limit)
	}
	written, truncated := h.streamAuditExport(w, r, page, "audit-deployment")
	// An export narrowed to one team is recorded in that team's trail as well,
	// so a team owner can see that the deployment read their record. An
	// unfiltered one has no team to name and stays deployment-scoped.
	h.cfg.Audit.Record(r.Context(), model.AuditEvent{
		TeamID:     filter.TeamID,
		ActorType:  model.AuditActorUser,
		ActorID:    adminID,
		Action:     model.AuditEventsExported,
		TargetType: "audit_event",
		Detail:     auditExportDetail(written, truncated),
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
func (h *Handler) streamAuditExport(w http.ResponseWriter, r *http.Request, page auditPageFunc, name string) (written int, truncated bool) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "jsonl" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "format must be csv or jsonl")
		return 0, false
	}

	filename := fmt.Sprintf("%s-%s.%s", name, time.Now().UTC().Format("20060102T150405Z"), format)
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/x-ndjson")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	csvOut := csv.NewWriter(w)
	jsonOut := json.NewEncoder(w)
	if format == "csv" {
		_ = csvOut.Write(auditExportHeader)
	}

	var cursor model.AuditCursor
	for written < auditExportMax {
		limit := min(auditExportPage, auditExportMax-written)
		events, err := page(r.Context(), cursor, limit)
		if err != nil {
			slog.Error("audit export failed partway", "err", err, "written", written)
			break
		}
		if len(events) == 0 {
			break
		}
		for _, event := range events {
			if format == "csv" {
				_ = csvOut.Write(auditExportRow(event))
			} else {
				_ = jsonOut.Encode(event)
			}
			written++
		}
		last := events[len(events)-1]
		cursor = model.AuditCursor{CreatedAt: last.CreatedAt, ID: last.ID}

		csvOut.Flush()
		if flusher != nil {
			flusher.Flush()
		}
		if len(events) < limit {
			break
		}
	}
	csvOut.Flush()
	return written, written >= auditExportMax
}

func auditExportRow(e model.AuditEvent) []string {
	return []string{
		e.AuditEventID,
		// RFC 3339 rather than the stored Unix seconds: the CSV is meant to be
		// opened, and a column of epoch integers is not readable by the person
		// opening it. The JSONL form keeps the raw field.
		time.Unix(e.CreatedAt, 0).UTC().Format(time.RFC3339),
		e.TeamID,
		e.ActorType,
		e.ActorID,
		e.Action,
		e.TargetType,
		e.TargetID,
		e.Detail,
	}
}

func auditExportDetail(written int, truncated bool) string {
	detail := strconv.Itoa(written) + " events"
	if truncated {
		detail += " (truncated at the export cap)"
	}
	return detail
}

// adminAuditFilter reads the deployment-scoped filters from a query. It is
// shared by the search and the export so the two cannot answer differently.
func adminAuditFilter(q url.Values) model.AuditFilter {
	filter := model.AuditFilter{
		TeamID:  q.Get("team_id"),
		ActorID: q.Get("actor_id"),
		Action:  q.Get("action"),
		Since:   parseUnixParam(q.Get("since")),
		Until:   parseUnixParam(q.Get("until")),
	}
	// team_id=none asks for the events no team-scoped reader can ever see:
	// logins, grants, account actions. An empty team_id already means "any
	// team", so this needs a spelling of its own.
	if filter.TeamID == "none" {
		filter.TeamID = ""
		filter.WithoutTeam = true
	}
	return filter
}
