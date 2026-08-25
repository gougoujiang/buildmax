package admin

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
)

// teamAuditExport drives the team-scoped export as one user.
//
// It builds its own mux rather than borrowing the authorization matrix's,
// because this test is about what the response body contains and the matrix's
// mux is deliberately seeded with an empty trail.
func exportBody(t *testing.T, mux *http.ServeMux, query string) *httptest.ResponseRecorder {
	t.Helper()
	rec := adminRequestAs(t, mux, adminCase{"GET", "/api/admin/audit-events/export" + query}, adminUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET export%s got %d: %s", query, rec.Code, rec.Body.String())
	}
	return rec
}

// The export is the trail in a file, so it has to carry every event the search
// would return — not the first page of them.
func TestAdminAuditExportCarriesEveryEvent(t *testing.T) {
	mux, audits := auditSearchMux(t)
	before := len(audits.Events)

	rec := exportBody(t, mux, "?format=csv")

	rows, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	// Header plus every seeded event. The export's own audit record is written
	// after the stream, so it is deliberately not in the file it describes.
	if len(rows) != before+1 {
		t.Fatalf("csv has %d rows, want %d (header plus %d events)", len(rows), before+1, before)
	}
	if rows[0][0] != "audit_event_id" || rows[0][5] != "action" {
		t.Errorf("header = %v", rows[0])
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/csv") {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment", got)
	}
}

// The export takes the same filters as the search it belongs to. If they ever
// diverge, an operator's file stops matching the screen they took it from.
func TestAdminAuditExportHonoursTheSameFilters(t *testing.T) {
	mux, _ := auditSearchMux(t)

	rec := exportBody(t, mux, "?format=jsonl&team_id=tm_one")

	var lines []coreaudit.Event
	for line := range strings.SplitSeq(strings.TrimSpace(rec.Body.String()), "\n") {
		if line == "" {
			continue
		}
		var event coreaudit.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse jsonl line %q: %v", line, err)
		}
		lines = append(lines, event)
	}
	if len(lines) != 1 {
		t.Fatalf("exported %d events, want the 1 in tm_one", len(lines))
	}
	if lines[0].TeamID != "tm_one" {
		t.Errorf("exported an event from %q", lines[0].TeamID)
	}
}

// team_id=none is the only way to reach the events that have no team at all,
// and the export has to spell it the same way the search does.
func TestAdminAuditExportReachesTheEventsWithNoTeam(t *testing.T) {
	mux, _ := auditSearchMux(t)

	rec := exportBody(t, mux, "?format=jsonl&team_id=none")

	count := 0
	for line := range strings.SplitSeq(strings.TrimSpace(rec.Body.String()), "\n") {
		if line == "" {
			continue
		}
		var event coreaudit.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse jsonl: %v", err)
		}
		if event.TeamID != "" {
			t.Errorf("team_id=none returned an event scoped to %q", event.TeamID)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("exported %d deployment-scoped events, want 2", count)
	}
}

// Reading the whole record is itself an action on it. An export that left no
// trace would be the one way to consult the trail without appearing in it.
func TestAdminAuditExportIsItselfRecorded(t *testing.T) {
	mux, audits := auditSearchMux(t)

	exportBody(t, mux, "?format=csv")

	var recorded *coreaudit.Event
	for i := range audits.Events {
		if audits.Events[i].Action == coreaudit.EventsExported {
			recorded = &audits.Events[i]
		}
	}
	if recorded == nil {
		t.Fatal("the export recorded nothing")
	}
	if recorded.ActorID != adminUser {
		t.Errorf("actor = %q, want the administrator who took it", recorded.ActorID)
	}
	if !strings.Contains(recorded.Detail, "events") {
		t.Errorf("detail = %q, want it to name what left", recorded.Detail)
	}
}

// An export narrowed to one team is recorded in that team's trail too, so its
// owner can see that the deployment read their record.
func TestAdminAuditExportOfOneTeamIsVisibleToThatTeam(t *testing.T) {
	mux, audits := auditSearchMux(t)

	exportBody(t, mux, "?format=csv&team_id=tm_one")

	for _, event := range audits.Events {
		if event.Action == coreaudit.EventsExported {
			if event.TeamID != "tm_one" {
				t.Errorf("recorded against team %q, want tm_one", event.TeamID)
			}
			return
		}
	}
	t.Fatal("the export recorded nothing")
}

// An unknown format is refused before anything is written, because a response
// that has already started streaming cannot be turned into an error.
func TestAuditExportRefusesAnUnknownFormat(t *testing.T) {
	mux, _ := auditSearchMux(t)

	rec := adminRequestAs(t, mux, adminCase{"GET", "/api/admin/audit-events/export?format=xlsx"}, adminUser)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

// The CSV is meant to be opened by a person, so the timestamp column is a date
// rather than the epoch integer the JSONL form keeps.
func TestAuditExportCSVWritesReadableTimes(t *testing.T) {
	mux, _ := auditSearchMux(t)

	rec := exportBody(t, mux, "?format=csv")

	rows, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(rows) < 2 {
		t.Fatal("no data rows")
	}
	if !strings.Contains(rows[1][1], "T") || !strings.HasSuffix(rows[1][1], "Z") {
		t.Errorf("created_at = %q, want an RFC 3339 timestamp", rows[1][1])
	}
}

// The team-scoped export must never become a way to read another team's trail,
// however the request is spelled.
