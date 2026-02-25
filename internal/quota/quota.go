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
