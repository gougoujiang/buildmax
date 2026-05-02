package db

import (
	"buildmax/internal/core/model"
	"context"
	"errors"

	"gorm.io/gorm"
)

// GetQuotaTier returns the tier limits by tier name, or (nil, nil) when not found.
func (s *Store) GetQuotaTier(ctx context.Context, tierName string) (*model.QuotaTier, error) {
	var t model.QuotaTier
	err := s.db.WithContext(ctx).Where("tier_name = ?", tierName).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// SeedDefaultQuotaTiers inserts free_trial and pro tiers if the quota_tier table is empty.
func (s *Store) SeedDefaultQuotaTiers(ctx context.Context) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.QuotaTier{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	defaults := []model.QuotaTier{
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
