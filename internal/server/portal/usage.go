package portal

import (
	"net/http"

	"buildmax/internal/server/httputil"
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
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "usage not available")
		return
	}
	info, err := h.cfg.QuotaChecker.GetUsage(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "usage")
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
	httputil.WriteJSON(w, http.StatusOK, resp)
}
