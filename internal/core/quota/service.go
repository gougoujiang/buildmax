package quota

import (
	"context"
	"time"

	"buildmax/internal/core/model"
)

// QuotaService enforces team-scoped quota (run count and token limits) using tier limits from the store.
type QuotaService struct {
	TeamStore   model.TeamStore
	UsageReader model.UsageInWindowReader
	TierStore   model.QuotaTierStore
	DefaultTier string
	clock       func() time.Time
}

// UsageInfo is a snapshot of a team's usage and tier limits for display.
type UsageInfo struct {
	RunCount           int
	TotalTokens        int
	TierName           string
	PeriodDays         int
	MaxRunsPerPeriod   *int // nil when tier unknown or not found
	MaxTokensPerPeriod *int
}

const defaultUsagePeriodDays = 30

func (c *QuotaService) now() time.Time {
	if c.clock != nil {
		return c.clock()
	}
	return time.Now()
}

// GetUsage returns the current team's usage and tier info in the same rolling window used by Check.
// When team or tier is not found, returns usage for a default 30-day window with limits nil.
func (c *QuotaService) GetUsage(ctx context.Context, teamID string) (*UsageInfo, error) {
	team, err := c.TeamStore.GetTeam(ctx, teamID)
	if err != nil {
		return &UsageInfo{}, nil
	}
	if team == nil {
		return &UsageInfo{}, nil
	}
	tierName := team.QuotaTier
	if tierName == "" {
		tierName = c.DefaultTier
	}
	now := c.now().Unix()

	// Resolve tier limits; if not found, still return usage for default window with limits nil
	tier, err := c.TierStore.GetQuotaTier(ctx, tierName)
	if err != nil {
		return &UsageInfo{}, err
	}
	if tier == nil {
		periodDays := defaultUsagePeriodDays
		since := now - int64(periodDays)*86400
		runCount, totalTokens, err := c.UsageReader.TeamUsageInWindow(ctx, teamID, since, now)
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
	runCount, totalTokens, err := c.UsageReader.TeamUsageInWindow(ctx, teamID, since, now)
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

// Check returns whether the team is allowed to add addRuns and addTokens. If not allowed, reason is the 429 message.
func (c *QuotaService) Check(ctx context.Context, teamID string, addRuns, addTokens int) (allowed bool, reason string) {
	team, err := c.TeamStore.GetTeam(ctx, teamID)
	if err != nil || team == nil {
		return true, "" // backward compatibility: no team or error => allow
	}
	tierName := team.QuotaTier
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
	now := c.now().Unix()
	periodSec := int64(tier.PeriodDays) * 86400
	since := now - periodSec
	runCount, totalTokens, err := c.UsageReader.TeamUsageInWindow(ctx, teamID, since, now)
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
