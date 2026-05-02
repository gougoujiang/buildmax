package portalapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"buildmax/internal/core/quota"
	"buildmax/internal/infra/db"
	"buildmax/internal/testutil"
)

func TestUsageHandler(t *testing.T) {
	secret := "test-usage-secret"
	userID := "u1"
	teamID := "tm_personal_u1"
	teamStore := &testutil.MockTeamStore{
		Teams: []db.Team{
			{TeamID: teamID, Name: "My Space", PersonalForUserID: testutil.PtrString(userID), QuotaTier: "free_trial", CreatedBy: userID, CreatedAt: time.Now().Unix()},
		},
		Members: []db.TeamMember{
			{TeamID: teamID, UserID: userID, Role: db.TeamRoleOwner, CreatedAt: time.Now().Unix()},
		},
	}
	usageReader := &testutil.MockUsageReader{RunCount: 2, TotalTokens: 5000}
	tierStore := &testutil.MockTierStore{
		Tier: &db.QuotaTier{
			TierName:           "free_trial",
			MaxRunsPerPeriod:   10,
			MaxTokensPerPeriod: 100_000,
			PeriodDays:         30,
		},
	}
	checker := quota.NewChecker(teamStore, usageReader, tierStore, "free_trial")

	tests := []struct {
		name        string
		authHeader  string
		path        string
		checker     *quota.Checker
		jwtSecret   string
		wantStatus  int
		wantBodyHas string
		checkBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "no auth returns 401",
			authHeader:  "",
			path:        "/api/usage",
			checker:     checker,
			jwtSecret:   secret,
			wantStatus:  http.StatusUnauthorized,
			wantBodyHas: "unauthorized",
		},
		{
			name:        "no QuotaChecker returns 503",
			authHeader:  "Bearer " + testutil.SignJWT(userID, secret),
			path:        "/api/usage",
			checker:     nil,
			jwtSecret:   secret,
			wantStatus:  http.StatusServiceUnavailable,
			wantBodyHas: "usage not available",
		},
		{
			name:       "legacy usage route returns personal team usage",
			authHeader: "Bearer " + testutil.SignJWT(userID, secret),
			path:       "/api/usage",
			checker:    checker,
			jwtSecret:  secret,
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				assertUsageBody(t, body)
			},
		},
		{
			name:       "team usage route returns team usage",
			authHeader: "Bearer " + testutil.SignJWT(userID, secret),
			path:       "/api/teams/" + teamID + "/usage",
			checker:    checker,
			jwtSecret:  secret,
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				assertUsageBody(t, body)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(Config{
				JWTSecret:    tt.jwtSecret,
				TeamStore:    teamStore,
				QuotaChecker: tt.checker,
			})
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			body := rec.Body.Bytes()
			if tt.wantBodyHas != "" && !strings.Contains(string(body), tt.wantBodyHas) {
				t.Errorf("body %q does not contain %q", body, tt.wantBodyHas)
			}
			if tt.checkBody != nil {
				tt.checkBody(t, body)
			}
		})
	}
}

func assertUsageBody(t *testing.T, body []byte) {
	t.Helper()
	var resp struct {
		RunCount           int    `json:"run_count"`
		TotalTokens        int    `json:"total_tokens"`
		TierName           string `json:"tier"`
		PeriodDays         int    `json:"period_days"`
		MaxRunsPerPeriod   *int   `json:"max_runs_per_period,omitempty"`
		MaxTokensPerPeriod *int   `json:"max_tokens_per_period,omitempty"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.RunCount != 2 || resp.TotalTokens != 5000 {
		t.Errorf("run_count=%d total_tokens=%d, want 2 and 5000", resp.RunCount, resp.TotalTokens)
	}
	if resp.TierName != "free_trial" || resp.PeriodDays != 30 {
		t.Errorf("tier=%q period_days=%d, want free_trial and 30", resp.TierName, resp.PeriodDays)
	}
	if resp.MaxRunsPerPeriod == nil || *resp.MaxRunsPerPeriod != 10 {
		t.Errorf("max_runs_per_period=%v, want 10", resp.MaxRunsPerPeriod)
	}
	if resp.MaxTokensPerPeriod == nil || *resp.MaxTokensPerPeriod != 100_000 {
		t.Errorf("max_tokens_per_period=%v, want 100000", resp.MaxTokensPerPeriod)
	}
}
