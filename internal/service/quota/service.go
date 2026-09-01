package quota

import (
	"context"
	"fmt"
	"time"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	corequota "github.com/gougoujiang/buildmax/internal/core/quota"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
)

// Service enforces team-scoped quota (run count and token limits) using tier limits from the store.
type Service struct {
	TeamStore   coreteam.Store
	UsageReader corequota.UsageInWindowReader
	TierStore   corequota.TierStore
	// StorageReader reports the bytes a team's artifacts hold. Nil leaves the
	// storage dimension unenforced and unreported, which is what a deployment
	// without artifact storage has.
	StorageReader corequota.StorageReader
	DefaultTier   string
	// Audit records a team approaching or reaching its limits. Nil records
	// nothing, so a deployment without a database still enforces quota — the
	// enforcement is the point, and the record of it is what a team admin reads
	// afterwards. It is the full store rather than a writer because the events
	// are deduplicated against what is already there; see alert.go.
	Audit coreaudit.Store
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
	// StorageBytes is what the team's artifacts hold now, not a windowed
	// total. Nil when this deployment has no artifact storage to read.
	StorageBytes    *int64
	MaxStorageBytes *int64
}

const defaultUsagePeriodDays = 30

func (c *Service) now() time.Time {
	if c.clock != nil {
		return c.clock()
	}
	return time.Now()
}

// GetUsage returns the current team's usage and tier info in the same rolling
// window used by Check. When team or tier is not found, returns usage for a
// default 30-day window with limits nil; a read that fails is an error, because
// a zeroed snapshot reads as "this team has used nothing".
func (c *Service) GetUsage(ctx context.Context, teamID string) (*UsageInfo, error) {
	team, err := c.TeamStore.GetTeam(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("read team %s: %w", teamID, err)
	}
	if team == nil {
		return &UsageInfo{}, nil
	}
	tierName := team.QuotaTier
	if tierName == "" {
		tierName = c.DefaultTier
	}
	now := c.now().UTC()

	// Resolve tier limits; if not found, still return usage for default window with limits nil
	tier, err := c.TierStore.GetQuotaTier(ctx, tierName)
	if err != nil {
		return nil, fmt.Errorf("read quota tier %s: %w", tierName, err)
	}
	if tier == nil {
		periodDays := defaultUsagePeriodDays
		since := now.Add(-time.Duration(periodDays) * 24 * time.Hour)
		runCount, totalTokens, err := c.UsageReader.TeamUsageInWindow(ctx, teamID, since, now)
		if err != nil {
			return nil, fmt.Errorf("read team %s usage: %w", teamID, err)
		}
		return &UsageInfo{
			RunCount:    runCount,
			TotalTokens: totalTokens,
			TierName:    tierName,
			PeriodDays:  periodDays,
		}, nil
	}

	since := now.Add(-time.Duration(tier.PeriodDays) * 24 * time.Hour)
	runCount, totalTokens, err := c.UsageReader.TeamUsageInWindow(ctx, teamID, since, now)
	if err != nil {
		return nil, fmt.Errorf("read team %s usage: %w", teamID, err)
	}
	maxRuns := tier.MaxRunsPerPeriod
	maxTokens := tier.MaxTokensPerPeriod
	info := &UsageInfo{
		RunCount:           runCount,
		TotalTokens:        totalTokens,
		TierName:           tier.TierName,
		PeriodDays:         tier.PeriodDays,
		MaxRunsPerPeriod:   &maxRuns,
		MaxTokensPerPeriod: &maxTokens,
	}
	// Read only where there is something to read. A deployment with no artifact
	// storage reports no storage rather than zero, which would read as a team
	// holding nothing.
	if c.StorageReader != nil {
		held, err := c.StorageReader.TeamArtifactBytes(ctx, teamID)
		if err != nil {
			return nil, fmt.Errorf("read team %s storage: %w", teamID, err)
		}
		maxStorage := tier.MaxStorageBytes
		info.StorageBytes = &held
		info.MaxStorageBytes = &maxStorage
	}
	return info, nil
}

// CheckStorage reports whether the team may hold addBytes more.
//
// Separate from Check because storage is a stock, not a rate: there is no
// window, PeriodDays does not apply, and the answer changes when an artifact is
// deleted rather than when time passes. The admission rules are otherwise the
// same — an unreadable limit is an error rather than an allowance, and an
// absent one is no limit.
func (c *Service) CheckStorage(ctx context.Context, teamID string, addBytes int64) (allowed bool, reason string, err error) {
	if c.StorageReader == nil {
		return true, "", nil
	}
	team, err := c.TeamStore.GetTeam(ctx, teamID)
	if err != nil {
		return false, "", fmt.Errorf("read team %s: %w", teamID, err)
	}
	if team == nil {
		return true, "", nil
	}
	tierName := team.QuotaTier
	if tierName == "" {
		tierName = c.DefaultTier
	}
	if tierName == "" {
		return true, "", nil
	}
	tier, err := c.TierStore.GetQuotaTier(ctx, tierName)
	if err != nil {
		return false, "", fmt.Errorf("read quota tier %s: %w", tierName, err)
	}
	// A tier that sets no storage limit imposes none. This is what keeps a
	// deployment that never chose a storage policy from acquiring one when the
	// column appeared underneath it.
	if tier == nil || tier.MaxStorageBytes <= 0 {
		return true, "", nil
	}
	held, err := c.StorageReader.TeamArtifactBytes(ctx, teamID)
	if err != nil {
		return false, "", fmt.Errorf("read team %s storage: %w", teamID, err)
	}
	now := c.now().UTC()
	since := now.Add(-time.Duration(tier.PeriodDays) * 24 * time.Hour)
	if held+addBytes > tier.MaxStorageBytes {
		c.noteUsage(ctx, teamID, limitStorage, held, tier.MaxStorageBytes, since, true)
		return false, fmt.Sprintf("quota exceeded: storage limit, %d of %d bytes used", held, tier.MaxStorageBytes), nil
	}
	c.warnIfNear(ctx, teamID, limitStorage, held+addBytes, tier.MaxStorageBytes, since)
	return true, "", nil
}

// Check returns whether the team is allowed to add addRuns and addTokens. If not
// allowed, reason is the 429 message.
//
// A configured limit that cannot be read is an error, not an allowance. The two
// look the same to a caller that only asks "allowed?", and answering yes means a
// deployment whose database is unreachable serves unmetered work and records
// nothing about having done so. Absence of a limit still means no limit: a team
// with no record, no tier, or a tier that names nothing is admitted.
func (c *Service) Check(ctx context.Context, teamID string, addRuns, addTokens int) (allowed bool, reason string, err error) {
	team, err := c.TeamStore.GetTeam(ctx, teamID)
	if err != nil {
		return false, "", fmt.Errorf("read team %s: %w", teamID, err)
	}
	if team == nil {
		return true, "", nil // no team => no tier => no limit
	}
	tierName := team.QuotaTier
	if tierName == "" {
		tierName = c.DefaultTier
	}
	if tierName == "" {
		return true, "", nil // no tier => allow
	}
	tier, err := c.TierStore.GetQuotaTier(ctx, tierName)
	if err != nil {
		return false, "", fmt.Errorf("read quota tier %s: %w", tierName, err)
	}
	if tier == nil {
		return true, "", nil // the tier names nothing => no limit
	}
	now := c.now().UTC()
	since := now.Add(-time.Duration(tier.PeriodDays) * 24 * time.Hour)
	runCount, totalTokens, err := c.UsageReader.TeamUsageInWindow(ctx, teamID, since, now)
	if err != nil {
		return false, "", fmt.Errorf("read team %s usage: %w", teamID, err)
	}
	if runCount+addRuns > tier.MaxRunsPerPeriod {
		c.noteUsage(ctx, teamID, limitRuns, int64(runCount), int64(tier.MaxRunsPerPeriod), since, true)
		return false, "quota exceeded: run limit", nil
	}
	if totalTokens+addTokens > tier.MaxTokensPerPeriod {
		c.noteUsage(ctx, teamID, limitTokens, int64(totalTokens), int64(tier.MaxTokensPerPeriod), since, true)
		return false, "quota exceeded: token limit", nil
	}

	// Warn on the way past the threshold, on an admission that was allowed.
	// This is the only moment the crossing is visible: usage is a rolling
	// window with no period boundary for a sweep to notice it at.
	c.warnIfNear(ctx, teamID, limitRuns, int64(runCount+addRuns), int64(tier.MaxRunsPerPeriod), since)
	c.warnIfNear(ctx, teamID, limitTokens, int64(totalTokens+addTokens), int64(tier.MaxTokensPerPeriod), since)
	return true, "", nil
}

// warnIfNear records a threshold crossing, and does nothing below it.
func (c *Service) warnIfNear(ctx context.Context, teamID string, limit quotaLimit, used, max int64, windowStart time.Time) {
	if c.Audit == nil || max <= 0 {
		return
	}
	if float64(used) < quotaWarnThreshold*float64(max) {
		return
	}
	c.noteUsage(ctx, teamID, limit, used, max, windowStart, false)
}
