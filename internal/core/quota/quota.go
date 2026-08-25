package quota

import (
	"context"
	"time"
)

// Tier defines limits for a tier (e.g. free_trial, pro).
type Tier struct {
	TierName           string `json:"tier_name"`
	MaxRunsPerPeriod   int    `json:"max_runs_per_period"`
	MaxTokensPerPeriod int    `json:"max_tokens_per_period"`
	PeriodDays         int    `json:"period_days"`
}

// TierStore provides quota tier limits by tier name.
type TierStore interface {
	// GetQuotaTier returns the tier limits by tier name, or (nil, nil) when not found.
	GetQuotaTier(ctx context.Context, tierName string) (*Tier, error)
}

// UsageInWindowReader provides usage aggregation for a team in a time window.
type UsageInWindowReader interface {
	// TeamUsageInWindow returns run count and total tokens for the team in [sinceUnix, untilUnix].
	TeamUsageInWindow(ctx context.Context, teamID string, since, until time.Time) (runCount, totalTokens int, err error)
}
