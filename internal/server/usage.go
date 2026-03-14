package server

import (
	"net/http"
)

// usageResponse is the JSON body for GET /api/usage (snake_case).
type usageResponse struct {
	RunCount           int   `json:"run_count"`
	TotalTokens        int   `json:"total_tokens"`
	TierName           string `json:"tier"`
	PeriodDays         int   `json:"period_days"`
	MaxRunsPerPeriod   *int  `json:"max_runs_per_period,omitempty"`
	MaxTokensPerPeriod *int  `json:"max_tokens_per_period,omitempty"`
}

func (s *Server) usageHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r, s.cfg.Auth.JWTSecret)
	if !ok {
		return
	}
	if s.cfg.Auth.QuotaChecker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "usage not available")
		return
	}
	info, err := s.cfg.Auth.QuotaChecker.GetUsage(r.Context(), userID)
	if err != nil {
		writeInternalError(w, err, "handler", "usage")
		return
	}
	resp := usageResponse{
		RunCount:           info.RunCount,
		TotalTokens:        info.TotalTokens,
		TierName:           info.TierName,
		PeriodDays:         info.PeriodDays,
		MaxRunsPerPeriod:   info.MaxRunsPerPeriod,
		MaxTokensPerPeriod: info.MaxTokensPerPeriod,
	}
	writeJSON(w, http.StatusOK, resp)
}
