package db

import (
	"buildmax/internal/core/model"
	"context"
)

// TeamUsageInWindow returns run count and total tokens for the team in [sinceUnix, untilUnix].
// Runs: task_runs where the task's team_id = teamID and run created_at in window.
// Tokens: sum of run prompt+completion tokens for those runs, plus task title tokens for tasks created in the team in window.
func (s *Store) TeamUsageInWindow(ctx context.Context, teamID string, sinceUnix, untilUnix int64) (runCount, totalTokens int, err error) {
	// Run count: task_runs joined with task where task.team_id = teamID and task_run.created_at in [since, until]
	var runCnt int64
	err = s.db.WithContext(ctx).Model(&model.TaskRun{}).
		Joins("INNER JOIN task ON task.task_id = task_run.task_id AND task.team_id = ?", teamID).
		Where("task_run.created_at >= ? AND task_run.created_at <= ?", sinceUnix, untilUnix).
		Count(&runCnt).Error
	if err != nil {
		return 0, 0, err
	}
	runCount = int(runCnt)

	// Run tokens: sum (COALESCE(prompt_tokens,0) + COALESCE(completion_tokens,0)) for those runs
	var runTokens int
	err = s.db.WithContext(ctx).Model(&model.TaskRun{}).
		Select("COALESCE(SUM(COALESCE(prompt_tokens, 0) + COALESCE(completion_tokens, 0)), 0)").
		Joins("INNER JOIN task ON task.task_id = task_run.task_id AND task.team_id = ?", teamID).
		Where("task_run.created_at >= ? AND task_run.created_at <= ?", sinceUnix, untilUnix).
		Scan(&runTokens).Error
	if err != nil {
		return runCount, 0, err
	}

	// Task title tokens: sum (title_prompt_tokens + title_completion_tokens) for tasks created by user in window
	var titleTokens int
	err = s.db.WithContext(ctx).Model(&model.Task{}).
		Select("COALESCE(SUM(title_prompt_tokens + title_completion_tokens), 0)").
		Where("team_id = ? AND created_at >= ? AND created_at <= ?", teamID, sinceUnix, untilUnix).
		Scan(&titleTokens).Error
	if err != nil {
		return runCount, runTokens, err
	}

	totalTokens = runTokens + titleTokens
	return runCount, totalTokens, nil
}
