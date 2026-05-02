package portalapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"buildmax/internal/infra/db"
	"buildmax/internal/testutil"
)

const teamTestSecret = "team-test-secret"

func TestTeamHandlers(t *testing.T) {
	personalTeamID := "tm_personal_u1"
	sharedTeamID := "tm_shared"
	teamStore := &testutil.MockTeamStore{
		Teams: []db.Team{
			{TeamID: personalTeamID, Name: "My Space", PersonalForUserID: testutil.PtrString("u1"), CreatedBy: "u1", CreatedAt: 100, UpdatedAt: 100},
			{TeamID: sharedTeamID, Name: "Ops", CreatedBy: "u1", CreatedAt: 200, UpdatedAt: 200},
		},
		Members: []db.TeamMember{
			{TeamID: personalTeamID, UserID: "u1", Role: db.TeamRoleOwner, CreatedAt: 100},
			{TeamID: sharedTeamID, UserID: "u1", Role: db.TeamRoleOwner, CreatedAt: 200},
			{TeamID: sharedTeamID, UserID: "u2", Role: db.TeamRoleMember, CreatedAt: 201},
		},
	}
	userStore := &testutil.MockUserStore{
		ByEmail: map[string]*db.User{
			"u1@example.com": {UserID: "u1", Email: "u1@example.com", Name: "Alice"},
			"u2@example.com": {UserID: "u2", Email: "u2@example.com", Name: "Bob"},
			"u3@example.com": {UserID: "u3", Email: "u3@example.com", Name: "Carol"},
		},
		ByID: map[string]*db.User{
			"u1": {UserID: "u1", Email: "u1@example.com", Name: "Alice"},
			"u2": {UserID: "u2", Email: "u2@example.com", Name: "Bob"},
			"u3": {UserID: "u3", Email: "u3@example.com", Name: "Carol"},
		},
	}
	h := NewHandler(Config{
		JWTSecret: teamTestSecret,
		UserStore: userStore,
		TeamStore: teamStore,
	})
	mux := http.NewServeMux()
	h.Register(mux)

	t.Run("GET list teams", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams", nil)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", teamTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var out []teamResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("teams len = %d, want 2", len(out))
		}
		if out[0].ID != personalTeamID || out[1].ID != sharedTeamID {
			t.Fatalf("teams = %+v", out)
		}
	})

	t.Run("POST create team", func(t *testing.T) {
		body := bytes.NewBufferString(`{"name":"Design"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/teams", body)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", teamTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		var out teamResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode create team: %v", err)
		}
		if out.Name != "Design" {
			t.Fatalf("team name = %q, want %q", out.Name, "Design")
		}
		list, err := teamStore.ListTeamsByUser(req.Context(), "u1")
		if err != nil {
			t.Fatalf("ListTeamsByUser after create: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("teams len after create = %d, want 3", len(list))
		}
	})

	t.Run("GET team members", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+sharedTeamID+"/members", nil)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", teamTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var out []teamMemberResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode members: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("members len = %d, want 2", len(out))
		}
		if out[0].UserName == nil || *out[0].UserName != "Alice" {
			t.Fatalf("member[0].user_name = %+v, want Alice", out[0].UserName)
		}
		if out[1].UserEmail == nil || *out[1].UserEmail != "u2@example.com" {
			t.Fatalf("member[1].user_email = %+v, want u2@example.com", out[1].UserEmail)
		}
	})

	t.Run("POST add team member by owner", func(t *testing.T) {
		body := bytes.NewBufferString(`{"email":"u3@example.com"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+sharedTeamID+"/members", body)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", teamTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		var out teamMemberResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode add member: %v", err)
		}
		if out.UserEmail == nil || *out.UserEmail != "u3@example.com" {
			t.Fatalf("added member email = %+v, want u3@example.com", out.UserEmail)
		}
		list, err := teamStore.ListTeamMembers(req.Context(), sharedTeamID)
		if err != nil {
			t.Fatalf("ListTeamMembers after add: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("members len after add = %d, want 3", len(list))
		}
	})

	t.Run("POST add team member forbidden for non-owner", func(t *testing.T) {
		body := bytes.NewBufferString(`{"email":"u4@example.com"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+sharedTeamID+"/members", body)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u2", teamTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("POST add team member requires existing user", func(t *testing.T) {
		body := bytes.NewBufferString(`{"email":"missing@example.com"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+sharedTeamID+"/members", body)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", teamTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("DELETE remove team member by owner", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/teams/"+sharedTeamID+"/members/u2", nil)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", teamTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
		}
		list, err := teamStore.ListTeamMembers(req.Context(), sharedTeamID)
		if err != nil {
			t.Fatalf("ListTeamMembers after remove: %v", err)
		}
		for _, member := range list {
			if member.UserID == "u2" {
				t.Fatalf("u2 should have been removed")
			}
		}
	})

	t.Run("DELETE remove self forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/teams/"+sharedTeamID+"/members/u1", nil)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", teamTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("GET team members forbidden when not a member", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+sharedTeamID+"/members", nil)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u4", teamTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}
