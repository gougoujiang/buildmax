package quota

import (
	"context"
	"time"
)

// Tier defines limits for a tier (e.g. free_trial, pro).
//
// Two kinds of limit live here and they are not measured the same way. Runs and
// tokens are rates, spent over PeriodDays and forgotten as the window moves.
// Storage is a stock: bytes held now, released only when an artifact is deleted
// or expires. A tier that leaves a limit at zero does not impose it.
type Tier struct {
	TierName           string `json:"tier_name"`
	MaxRunsPerPeriod   int    `json:"max_runs_per_period"`
	MaxTokensPerPeriod int    `json:"max_tokens_per_period"`
	// MaxStorageBytes caps what the team's live artifacts may hold at once.
	// PeriodDays does not apply to it.
	MaxStorageBytes int64 `json:"max_storage_bytes"`
	PeriodDays      int   `json:"period_days"`
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

// StorageReader reports what a team currently holds.
//
// Separate from UsageInWindowReader because it takes no window: asking "how
// many bytes in the last 30 days" would answer a question nobody has, and a
// tier that limited it that way would let a team hold unbounded storage by
// waiting.
type StorageReader interface {
	// TeamArtifactBytes returns the bytes the team's live artifacts hold.
	TeamArtifactBytes(ctx context.Context, teamID string) (int64, error)
}
