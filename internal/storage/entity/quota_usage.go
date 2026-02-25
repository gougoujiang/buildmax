package entity

import (
	"context"
)

// UserUsageInWindow returns run count and total tokens for the user in [sinceUnix, untilUnix].
// Runs: chat_runs where the chat's created_by = userID and run created_at in window.
// Tokens: sum of run prompt+completion tokens for those runs, plus chat title tokens for chats created by user in window.
func (s *Store) UserUsageInWindow(ctx context.Context, userID string, sinceUnix, untilUnix int64) (runCount, totalTokens int, err error) {
	// Run count: chat_runs joined with chat where chat.created_by = userID and chat_run.created_at in [since, until]
	var runCnt int64
	err = s.db.WithContext(ctx).Model(&ChatRun{}).
		Joins("INNER JOIN chat ON chat.chat_id = chat_run.chat_id AND chat.created_by = ?", userID).
		Where("chat_run.created_at >= ? AND chat_run.created_at <= ?", sinceUnix, untilUnix).
		Count(&runCnt).Error
	if err != nil {
		return 0, 0, err
	}
	runCount = int(runCnt)

	// Run tokens: sum (COALESCE(prompt_tokens,0) + COALESCE(completion_tokens,0)) for those runs
	var runTokens int
	err = s.db.WithContext(ctx).Model(&ChatRun{}).
		Select("COALESCE(SUM(COALESCE(prompt_tokens, 0) + COALESCE(completion_tokens, 0)), 0)").
		Joins("INNER JOIN chat ON chat.chat_id = chat_run.chat_id AND chat.created_by = ?", userID).
		Where("chat_run.created_at >= ? AND chat_run.created_at <= ?", sinceUnix, untilUnix).
		Scan(&runTokens).Error
	if err != nil {
		return runCount, 0, err
	}

	// Chat title tokens: sum (title_prompt_tokens + title_completion_tokens) for chats created by user in window
	var titleTokens int
	err = s.db.WithContext(ctx).Model(&Chat{}).
		Select("COALESCE(SUM(title_prompt_tokens + title_completion_tokens), 0)").
		Where("created_by = ? AND created_at >= ? AND created_at <= ?", userID, sinceUnix, untilUnix).
		Scan(&titleTokens).Error
	if err != nil {
		return runCount, runTokens, err
	}

	totalTokens = runTokens + titleTokens
	return runCount, totalTokens, nil
}
