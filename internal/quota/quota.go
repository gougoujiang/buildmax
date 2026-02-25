package quota

import (
	"context"
	"time"

	"buildmax/internal/storage/entity"
)

// Checker enforces per-user quota (run count and token limits) using tier limits from the store.
type Checker struct {
	UserStore    entity.UserStore
	UsageReader  entity.UsageInWindowReader
	TierStore    entity.QuotaTierStore
	DefaultTier  string
	clock        func() time.Time
}

// NewChecker builds a Checker with the given dependencies. defaultTier is used when user.QuotaTier is empty.
func NewChecker(userStore entity.UserStore, usageReader entity.UsageInWindowReader, tierStore entity.QuotaTierStore, defaultTier string) *Checker {
	return &Checker{
		UserStore:   userStore,
		UsageReader: usageReader,
		TierStore:   tierStore,
		DefaultTier: defaultTier,
		clock:       time.Now,
	}
}

// UsageInfo is a snapshot of a user's usage and tier limits for display (e.g. settings page).
type UsageInfo struct {
	RunCount           int
	TotalTokens        int
	TierName           string
	PeriodDays         int
	MaxRunsPerPeriod   *int // nil when tier unknown or not found
	MaxTokensPerPeriod *int
}

const defaultUsagePeriodDays = 30

// GetUsage returns the current user's usage and tier info in the same rolling window used by Check.
// When user or tier is not found, returns usage for a default 30-day window with limits nil.
func (c *Checker) GetUsage(ctx context.Context, userID string) (*UsageInfo, error) {
	user, err := c.UserStore.GetUser(ctx, userID)
	if err != nil {
		return &UsageInfo{}, nil
	}
	if user == nil {
		return &UsageInfo{}, nil
	}
	tierName := user.QuotaTier
	if tierName == "" {
		tierName = c.DefaultTier
	}
	now := c.clock().Unix()

	// Resolve tier limits; if not found, still return usage for default window with limits nil
	tier, err := c.TierStore.GetQuotaTier(ctx, tierName)
	if err != nil {
		return &UsageInfo{}, err
	}
	if tier == nil {
		periodDays := defaultUsagePeriodDays
		since := now - int64(periodDays)*86400
		runCount, totalTokens, err := c.UsageReader.UserUsageInWindow(ctx, userID, since, now)
		if err != nil {
			return &UsageInfo{}, err
		}
		return &UsageInfo{
			RunCount:    runCount,
			TotalTokens: totalTokens,
			TierName:    tierName,
			PeriodDays:  periodDays,
		}, nil
	}

	periodSec := int64(tier.PeriodDays) * 86400
	since := now - periodSec
	runCount, totalTokens, err := c.UsageReader.UserUsageInWindow(ctx, userID, since, now)
	if err != nil {
		return nil, err
	}
	maxRuns := tier.MaxRunsPerPeriod
	maxTokens := tier.MaxTokensPerPeriod
	return &UsageInfo{
		RunCount:           runCount,
		TotalTokens:        totalTokens,
		TierName:           tier.TierName,
		PeriodDays:         tier.PeriodDays,
		MaxRunsPerPeriod:   &maxRuns,
		MaxTokensPerPeriod: &maxTokens,
	}, nil
}

// Check returns whether the user is allowed to add addRuns and addTokens. If not allowed, reason is the 429 message.
func (c *Checker) Check(ctx context.Context, userID string, addRuns, addTokens int) (allowed bool, reason string) {
	user, err := c.UserStore.GetUser(ctx, userID)
	if err != nil || user == nil {
		return true, "" // backward compatibility: no user or error => allow
	}
	tierName := user.QuotaTier
	if tierName == "" {
		tierName = c.DefaultTier
	}
	if tierName == "" {
		return true, "" // no tier => allow
	}
	tier, err := c.TierStore.GetQuotaTier(ctx, tierName)
	if err != nil || tier == nil {
		return true, "" // unknown tier => allow (no limit)
	}
	now := c.clock().Unix()
	periodSec := int64(tier.PeriodDays) * 86400
	since := now - periodSec
	runCount, totalTokens, err := c.UsageReader.UserUsageInWindow(ctx, userID, since, now)
	if err != nil {
		return true, "" // aggregation error => allow to avoid blocking
	}
	if runCount+addRuns > tier.MaxRunsPerPeriod {
		return false, "quota exceeded: run limit"
	}
	if totalTokens+addTokens > tier.MaxTokensPerPeriod {
		return false, "quota exceeded: token limit"
	}
	return true, ""
}
