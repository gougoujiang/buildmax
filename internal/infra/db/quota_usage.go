package db

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// TeamUsageInWindow returns run count and total tokens for the team in [since, until].
// Runs: task_runs where the task's team = teamID and run created_at in window.
// Tokens: sum of run prompt+completion tokens for those runs, plus task title tokens for tasks created in the team in window.
//
// The team's handle is resolved once, at the top. Everything after it is a
// numeric comparison: this is the hottest aggregation in the deployment and it
// no longer touches a string.
func (s *Store) TeamUsageInWindow(ctx context.Context, teamID string, since, until time.Time) (runCount, totalTokens int, err error) {
	teamKey, err := lookupKey(ctx, s.db, "team", teamID)
	if errors.Is(err, model.ErrNotFound) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}

	var runCnt int64
	err = s.db.WithContext(ctx).Model(&taskRunRow{}).
		Joins("INNER JOIN task ON task.id = task_run.task_id AND task.team_id = ?", teamKey).
		Where("task_run.created_at >= ? AND task_run.created_at <= ?", since, until).
		Count(&runCnt).Error
	if err != nil {
		return 0, 0, err
	}
	runCount = int(runCnt)

	var runTokens int
	err = s.db.WithContext(ctx).Model(&taskRunRow{}).
		Select("COALESCE(SUM(COALESCE(prompt_tokens, 0) + COALESCE(completion_tokens, 0)), 0)").
		Joins("INNER JOIN task ON task.id = task_run.task_id AND task.team_id = ?", teamKey).
		Where("task_run.created_at >= ? AND task_run.created_at <= ?", since, until).
		Scan(&runTokens).Error
	if err != nil {
		return runCount, 0, err
	}

	// Title generation is billed to the team too: it is a model call the team's
	// work caused, even though no run records it.
	var titleTokens int
	err = s.db.WithContext(ctx).Model(&taskRow{}).
		Select("COALESCE(SUM(title_prompt_tokens + title_completion_tokens), 0)").
		Where("team_id = ? AND created_at >= ? AND created_at <= ?", teamKey, since, until).
		Scan(&titleTokens).Error
	if err != nil {
		return runCount, runTokens, err
	}

	return runCount, runTokens + titleTokens, nil
}
