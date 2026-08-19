package quota

import (
	"context"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// QuotaService enforces team-scoped quota (run count and token limits) using tier limits from the store.
type QuotaService struct {
	TeamStore   model.TeamStore
	UsageReader model.UsageInWindowReader
	TierStore   model.QuotaTierStore
	DefaultTier string
	// Audit records a team approaching or reaching its limits. Nil records
	// nothing, so a deployment without a database still enforces quota — the
	// enforcement is the point, and the record of it is what a team admin reads
	// afterwards. It is the full store rather than a writer because the events
	// are deduplicated against what is already there; see alert.go.
	Audit model.AuditStore
	clock func() time.Time
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
		c.noteUsage(ctx, teamID, limitRuns, runCount, tier.MaxRunsPerPeriod, since, true)
		return false, "quota exceeded: run limit"
	}
	if totalTokens+addTokens > tier.MaxTokensPerPeriod {
		c.noteUsage(ctx, teamID, limitTokens, totalTokens, tier.MaxTokensPerPeriod, since, true)
		return false, "quota exceeded: token limit"
	}

	// Warn on the way past the threshold, on an admission that was allowed.
	// This is the only moment the crossing is visible: usage is a rolling
	// window with no period boundary for a sweep to notice it at.
	c.warnIfNear(ctx, teamID, limitRuns, runCount+addRuns, tier.MaxRunsPerPeriod, since)
	c.warnIfNear(ctx, teamID, limitTokens, totalTokens+addTokens, tier.MaxTokensPerPeriod, since)
	return true, ""
}

// warnIfNear records a threshold crossing, and does nothing below it.
func (c *QuotaService) warnIfNear(ctx context.Context, teamID string, limit quotaLimit, used, max int, windowStart int64) {
	if c.Audit == nil || max <= 0 {
		return
	}
	if float64(used) < quotaWarnThreshold*float64(max) {
		return
	}
	c.noteUsage(ctx, teamID, limit, used, max, windowStart, false)
}
