package portal

import (
	"net/http"
)

type usageResponse struct {
	RunCount           int   `json:"run_count"`
	TotalTokens        int   `json:"total_tokens"`
	TierName           string `json:"tier"`
	PeriodDays         int   `json:"period_days"`
	MaxRunsPerPeriod   *int  `json:"max_runs_per_period,omitempty"`
	MaxTokensPerPeriod *int  `json:"max_tokens_per_period,omitempty"`
}

func (h *Handler) usageHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r, h.cfg.JWTSecret)
	if !ok {
		return
	}
	if h.cfg.QuotaChecker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "usage not available")
		return
	}
	info, err := h.cfg.QuotaChecker.GetUsage(r.Context(), userID)
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
