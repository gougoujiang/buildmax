// Package auditexport streams the audit trail as CSV.
//
// Shared because two routes present the same rows: a team exports its own
// events and an administrator exports every team's. The columns, the row cap,
// and what a truncated export tells the caller must be one answer, not two that
// happen to agree.
package auditexport

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
type PageFunc func(ctx context.Context, after model.AuditCursor, limit int) ([]model.AuditEvent, error)

// exportAuditEventsHandler serves
// GET /api/teams/{team_id}/audit-events/export.
//
// Owner only, the same reader as the trail itself: an export is the trail, in a
// file. It carries the whole of a team's trail rather than a filtered slice —
// the reason to export is to keep or examine the record elsewhere, and a filter
// applied on the way out is a decision the file cannot show it made.
// Stream writes the CSV response and reports what it wrote.
func Stream(w http.ResponseWriter, r *http.Request, page PageFunc, name string) (written int, truncated bool) {
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
				_ = csvOut.Write(exportRow(event))
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

func exportRow(e model.AuditEvent) []string {
	return []string{
		e.ID,
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

func Detail(written int, truncated bool) string {
	detail := strconv.Itoa(written) + " events"
	if truncated {
		detail += " (truncated at the export cap)"
	}
	return detail
}

// adminAuditFilter reads the deployment-scoped filters from a query. It is
// shared by the search and the export so the two cannot answer differently.
// AdminFilter reads the deployment-scoped filters from a query.
func AdminFilter(q url.Values) model.AuditFilter {
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
