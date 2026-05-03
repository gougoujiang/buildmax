package db

import (
	"buildmax/internal/core/model"
	"context"
	"errors"

	"gorm.io/gorm"
)

type quotaTierRow struct {
	TierName           string `gorm:"column:tier_name;primaryKey;type:varchar(64)"`
	MaxRunsPerPeriod   int    `gorm:"column:max_runs_per_period;not null"`
	MaxTokensPerPeriod int    `gorm:"column:max_tokens_per_period;not null"`
	PeriodDays         int    `gorm:"column:period_days;not null"`
}

func (quotaTierRow) TableName() string { return "quota_tier" }

func toQuotaTier(row *quotaTierRow) *model.QuotaTier {
	if row == nil {
		return nil
	}
	return &model.QuotaTier{
		TierName:           row.TierName,
		MaxRunsPerPeriod:   row.MaxRunsPerPeriod,
		MaxTokensPerPeriod: row.MaxTokensPerPeriod,
		PeriodDays:         row.PeriodDays,
	}
}

// GetQuotaTier returns the tier limits by tier name, or (nil, nil) when not found.
func (s *Store) GetQuotaTier(ctx context.Context, tierName string) (*model.QuotaTier, error) {
	var t quotaTierRow
	err := s.db.WithContext(ctx).Where("tier_name = ?", tierName).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toQuotaTier(&t), nil
}

// SeedDefaultQuotaTiers inserts free_trial and pro tiers if the quota_tier table is empty.
func (s *Store) SeedDefaultQuotaTiers(ctx context.Context) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&quotaTierRow{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	defaults := []quotaTierRow{
		{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100_000, PeriodDays: 30},
		{TierName: "pro", MaxRunsPerPeriod: 1000, MaxTokensPerPeriod: 10_000_000, PeriodDays: 30},
	}
	for _, t := range defaults {
		if err := s.db.WithContext(ctx).Create(&t).Error; err != nil {
			return err
		}
	}
	return nil
}
