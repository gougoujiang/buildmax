package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"buildmax/internal/quota"
	"buildmax/internal/server/testutil"
	"buildmax/internal/storage/entity"
)

func TestUsageHandler(t *testing.T) {
	secret := "test-usage-secret"
	userID := "u1"
	userWithTier := &entity.User{
		UserID:    userID,
		Email:     "u1@test",
		QuotaTier: "free_trial",
		CreatedAt: time.Now().Unix(),
	}
	userStore := &testutil.MockUserStore{
		ByID: map[string]*entity.User{userID: userWithTier},
	}
	usageReader := &testutil.MockUsageReader{RunCount: 2, TotalTokens: 5000}
	tierStore := &testutil.MockTierStore{
		Tier: &entity.QuotaTier{
			TierName:           "free_trial",
			MaxRunsPerPeriod:    10,
			MaxTokensPerPeriod: 100_000,
			PeriodDays:         30,
		},
	}
	checker := quota.NewChecker(userStore, usageReader, tierStore, "free_trial")

	tests := []struct {
		name        string
		authHeader  string
		checker     *quota.Checker
		jwtSecret   string
		wantStatus  int
		wantBodyHas string
		checkBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "no auth returns 401",
			authHeader:  "",
			checker:     checker,
			jwtSecret:   secret,
			wantStatus:  http.StatusUnauthorized,
			wantBodyHas: "unauthorized",
		},
		{
			name:        "no QuotaChecker returns 503",
			authHeader:  "Bearer " + testutil.SignJWT(userID, secret),
			checker:     nil,
			jwtSecret:   secret,
			wantStatus:  http.StatusServiceUnavailable,
			wantBodyHas: "usage not available",
		},
		{
			name:       "valid JWT returns 200 with usage and limits",
			authHeader: "Bearer " + testutil.SignJWT(userID, secret),
			checker:    checker,
			jwtSecret:  secret,
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp struct {
					RunCount           int    `json:"run_count"`
					TotalTokens        int    `json:"total_tokens"`
					TierName           string `json:"tier"`
					PeriodDays         int   `json:"period_days"`
					MaxRunsPerPeriod   *int  `json:"max_runs_per_period,omitempty"`
					MaxTokensPerPeriod *int  `json:"max_tokens_per_period,omitempty"`
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(Config{
				JWTSecret:    tt.jwtSecret,
				QuotaChecker: tt.checker,
			})
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(http.MethodGet, "/api/usage", nil)
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
