package space

import (
	"encoding/json"
	"net/http"
	"testing"

	agentdef "github.com/icloudbb/buildmax/internal/core/agentdef"
	corespace "github.com/icloudbb/buildmax/internal/core/space"
	"github.com/icloudbb/buildmax/internal/mock"
	"github.com/icloudbb/buildmax/internal/testsupport"
)

const scheduleTestSecret = "schedule-test-secret"

// scheduleTestHandler wires a space with an owner and a plain member, one agent,
// and an empty schedule store. It returns the mux and the member's bearer token,
// because schedule management is a member-level action and the member is the
// interesting caller.
func scheduleTestHandler(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	spaceID := "tm_1"
	spaceStore := &mock.MockSpaceStore{
		Spaces: []corespace.Space{{ID: spaceID, Name: "Space", CreatedBy: "u1"}},
		Members: []corespace.Member{
			{SpaceID: spaceID, UserID: "u1", Role: corespace.RoleOwner},
			{SpaceID: spaceID, UserID: "u2", Role: corespace.RoleMember},
		},
	}
	agentStore := &mock.MockAgentStore{}
	agent, err := agentStore.CreateAgentInSpace(t.Context(), agentdef.CreateInput{
		SpaceID: spaceID, UserID: "u1",
		Def: agentdef.Definition{Name: "Summarizer", Description: "d", Instructions: "i"},
	})
	if err != nil {
		t.Fatalf("CreateAgentInSpace: %v", err)
	}
	h := New(Config{JWTSecret: scheduleTestSecret, Spaces: spaceStore, Agents: agentStore, Schedules: &mock.MockScheduleStore{}})
	mux := http.NewServeMux()
	h.Register(mux)
	token := "Bearer " + testsupport.SignJWT("u2", scheduleTestSecret)
	return mux, token, agent.ID
}

// A member walks a schedule through its whole lifecycle: create, read, list,
// pause, and delete.
func TestScheduleLifecycle(t *testing.T) {
	mux, token, agentID := scheduleTestHandler(t)
	base := "/api/spaces/tm_1/schedules"

	rec := doJSON(t, mux, http.MethodPost, base, token,
		`{"agent_id":"`+agentID+`","name":"nightly","input":"summarize","cron_expr":"0 9 * * *","timezone":"UTC"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var created ScheduleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if !created.Enabled || created.AgentID != agentID || created.CreatedBy != "u2" {
		t.Fatalf("created schedule = %+v, want enabled, the agent, and the member as creator", created)
	}
	if created.NextFireAt.IsZero() {
		t.Error("created schedule has no next fire time")
	}

	rec = doJSON(t, mux, http.MethodGet, base+"/"+created.ID, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, mux, http.MethodGet, base, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rec.Code)
	}
	var list scheduleListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Total != 1 || len(list.Schedules) != 1 {
		t.Fatalf("list total = %d, want 1", list.Total)
	}

	rec = doJSON(t, mux, http.MethodPatch, base+"/"+created.ID, token, `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var patched ScheduleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &patched); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if patched.Enabled {
		t.Error("schedule still enabled after a disable patch")
	}

	rec = doJSON(t, mux, http.MethodDelete, base+"/"+created.ID, token, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	rec = doJSON(t, mux, http.MethodGet, base+"/"+created.ID, token, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", rec.Code)
	}
}

// Create refuses an invalid cron expression, an agent that is not in the space,
// and an empty input — each as a 400, not a 500.
func TestCreateScheduleValidation(t *testing.T) {
	mux, token, agentID := scheduleTestHandler(t)
	base := "/api/spaces/tm_1/schedules"

	cases := []struct {
		name string
		body string
	}{
		{"bad cron", `{"agent_id":"` + agentID + `","input":"x","cron_expr":"nonsense","timezone":"UTC"}`},
		{"unknown agent", `{"agent_id":"a_nope","input":"x","cron_expr":"0 9 * * *","timezone":"UTC"}`},
		{"empty input", `{"agent_id":"` + agentID + `","input":"","cron_expr":"0 9 * * *","timezone":"UTC"}`},
		{"bad timezone", `{"agent_id":"` + agentID + `","input":"x","cron_expr":"0 9 * * *","timezone":"Mars/Phobos"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := doJSON(t, mux, http.MethodPost, base, token, c.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// A schedule id from another space is reported as not found, not read across the
// space boundary.
func TestGetScheduleFromAnotherSpaceIsNotFound(t *testing.T) {
	mux, token, agentID := scheduleTestHandler(t)
	base := "/api/spaces/tm_1/schedules"
	rec := doJSON(t, mux, http.MethodPost, base, token,
		`{"agent_id":"`+agentID+`","input":"x","cron_expr":"0 9 * * *","timezone":"UTC"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", rec.Code)
	}
	var created ScheduleResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	// The same id under a space the caller does not belong to is refused before
	// any lookup (403), never leaked as 200.
	rec = doJSON(t, mux, http.MethodGet, "/api/spaces/tm_other/schedules/"+created.ID, token, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-space get = %d, want 403", rec.Code)
	}
}
