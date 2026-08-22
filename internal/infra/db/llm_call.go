package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

// llmCallRow is the managed call ledger. It stores accounting and diagnostic
// metadata only — no prompts, tool payloads, or generated content.
type llmCallRow struct {
	ID           uint    `gorm:"primaryKey;autoIncrement"`
	PublicID     string  `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_llm_call_public_id;not null"`
	ClientCallID *string `gorm:"type:varchar(128);uniqueIndex:idx_llm_call_client,priority:2"`

	// The composite unique index leads with team_id, so team-scoped lookups do
	// not need a second index on this column.
	TeamID    uint64  `gorm:"column:team_id;not null;uniqueIndex:idx_llm_call_client,priority:1"`
	UserID    *uint64 `gorm:"column:user_id;index"`
	TaskRunID *uint64 `gorm:"column:task_run_id;index"`

	// SessionID names a file under a run's BUILDMAX_HOME, not a row.
	Surface   string  `gorm:"type:varchar(32)"`
	SessionID *string `gorm:"type:varchar(64)"`
	TaskID    *uint64 `gorm:"column:task_id;index"`

	// Alias and TargetID name a catalog entry whose namespace may be owned by
	// configuration rather than by llm_model, so neither becomes a reference.
	Alias         string `gorm:"type:varchar(64)"`
	TargetID      string `gorm:"type:varchar(64);not null"`
	ProviderType  string `gorm:"type:varchar(32);not null"`
	UpstreamModel string `gorm:"type:varchar(128);not null"`
	Streaming     bool   `gorm:"not null;default:false"`

	AcceptedAt        int64  `gorm:"not null;index"`
	UpstreamStartedAt *int64 `gorm:""`
	FirstDeltaAt      *int64 `gorm:""`
	CompletedAt       *int64 `gorm:""`

	Status     string  `gorm:"type:varchar(16);not null;index"`
	ErrorClass *string `gorm:"type:varchar(64)"`
	Attempts   int     `gorm:"not null;default:0"`

	PromptTokens     *int   `gorm:""`
	CompletionTokens *int   `gorm:""`
	TotalTokens      *int   `gorm:""`
	CacheReadTokens  *int   `gorm:""`
	CacheWriteTokens *int   `gorm:""`
	UsageSource      string `gorm:"type:varchar(16)"`
}

func (llmCallRow) TableName() string { return "llm_call" }

// llmCallReadRow is the row plus the handles its references resolve to. A
// pointer field is one a LEFT JOIN may leave NULL.
type llmCallReadRow struct {
	Row             llmCallRow `gorm:"embedded"`
	TeamPublicID    string     `gorm:"column:team_public_id"`
	UserPublicID    *string    `gorm:"column:user_public_id"`
	TaskPublicID    *string    `gorm:"column:task_public_id"`
	TaskRunPublicID *string    `gorm:"column:task_run_public_id"`
}

func (s *Store) llmCallSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&llmCallRow{}).
		Select("llm_call.*, t.public_id AS team_public_id, u.public_id AS user_public_id, " +
			"tk.public_id AS task_public_id, r.public_id AS task_run_public_id").
		Joins("INNER JOIN team t ON t.id = llm_call.team_id").
		Joins("LEFT JOIN `user` u ON u.id = llm_call.user_id").
		Joins("LEFT JOIN task tk ON tk.id = llm_call.task_id").
		Joins("LEFT JOIN task_run r ON r.id = llm_call.task_run_id")
}

func toLLMCall(row *llmCallReadRow) *model.LLMCall {
	if row == nil {
		return nil
	}
	out := &model.LLMCall{
		ID:                row.Row.PublicID,
		ClientCallID:      row.Row.ClientCallID,
		TeamID:            row.TeamPublicID,
		Surface:           row.Row.Surface,
		SessionID:         row.Row.SessionID,
		Alias:             row.Row.Alias,
		TargetID:          row.Row.TargetID,
		ProviderType:      row.Row.ProviderType,
		UpstreamModel:     row.Row.UpstreamModel,
		Streaming:         row.Row.Streaming,
		AcceptedAt:        row.Row.AcceptedAt,
		UpstreamStartedAt: row.Row.UpstreamStartedAt,
		FirstDeltaAt:      row.Row.FirstDeltaAt,
		CompletedAt:       row.Row.CompletedAt,
		Status:            row.Row.Status,
		ErrorClass:        row.Row.ErrorClass,
		Attempts:          row.Row.Attempts,
		PromptTokens:      row.Row.PromptTokens,
		CompletionTokens:  row.Row.CompletionTokens,
		TotalTokens:       row.Row.TotalTokens,
		CacheReadTokens:   row.Row.CacheReadTokens,
		CacheWriteTokens:  row.Row.CacheWriteTokens,
		UsageSource:       row.Row.UsageSource,
	}
	if row.Row.UserID != nil {
		user := derefPublicID(row.UserPublicID)
		out.UserID = &user
	}
	if row.Row.TaskID != nil {
		task := derefPublicID(row.TaskPublicID)
		out.TaskID = &task
	}
	if row.Row.TaskRunID != nil {
		run := derefPublicID(row.TaskRunPublicID)
		out.TaskRunID = &run
	}
	return out
}

// llmCallValues carries every column that is a value rather than a reference.
//
// It is separate from reference resolution so the mapping stays testable
// without a database: a column added to the model and forgotten here is the
// failure this split exists to keep catching.
func llmCallValues(call *model.LLMCall) *llmCallRow {
	if call == nil {
		return nil
	}
	return &llmCallRow{
		ClientCallID:      call.ClientCallID,
		Surface:           call.Surface,
		SessionID:         call.SessionID,
		Alias:             call.Alias,
		TargetID:          call.TargetID,
		ProviderType:      call.ProviderType,
		UpstreamModel:     call.UpstreamModel,
		Streaming:         call.Streaming,
		AcceptedAt:        call.AcceptedAt,
		UpstreamStartedAt: call.UpstreamStartedAt,
		FirstDeltaAt:      call.FirstDeltaAt,
		CompletedAt:       call.CompletedAt,
		Status:            call.Status,
		ErrorClass:        call.ErrorClass,
		Attempts:          call.Attempts,
		PromptTokens:      call.PromptTokens,
		CompletionTokens:  call.CompletionTokens,
		TotalTokens:       call.TotalTokens,
		CacheReadTokens:   call.CacheReadTokens,
		CacheWriteTokens:  call.CacheWriteTokens,
		UsageSource:       call.UsageSource,
	}
}

// toLLMCallRow resolves the call's references. It reports an error rather than
// dropping one: the ledger is an accounting record, and a call attributed to
// nothing is worse than a refused write.
func (s *Store) toLLMCallRow(ctx context.Context, call *model.LLMCall) (*llmCallRow, error) {
	teamKey, err := lookupKey(ctx, s.db, "team", call.TeamID)
	if err != nil {
		return nil, err
	}
	userKey, err := optionalKey(ctx, s.db, "user", call.UserID)
	if err != nil {
		return nil, err
	}
	taskKey, err := optionalKey(ctx, s.db, "task", call.TaskID)
	if err != nil {
		return nil, err
	}
	runKey, err := optionalKey(ctx, s.db, "task_run", call.TaskRunID)
	if err != nil {
		return nil, err
	}
	row := llmCallValues(call)
	row.TeamID = teamKey
	row.UserID = userKey
	row.TaskID = taskKey
	row.TaskRunID = runKey
	return row, nil
}

// OpenLLMCall records an accepted call before the upstream request starts, so a
// call that never returns still leaves evidence that it was attempted.
func (s *Store) OpenLLMCall(ctx context.Context, call *model.LLMCall) (*model.LLMCall, error) {
	if call == nil {
		return nil, errors.New("llm call is required")
	}
	stored := *call
	if stored.AcceptedAt == 0 {
		stored.AcceptedAt = time.Now().Unix()
	}
	if stored.Status == "" {
		stored.Status = model.LLMCallStatusAccepted
	}
	if stored.UsageSource == "" {
		stored.UsageSource = model.LLMUsageSourceUnavailable
	}
	row, err := s.toLLMCallRow(ctx, &stored)
	if err != nil {
		return nil, err
	}
	if err := createWithPublicID(ctx, s.db, "uq_llm_call_public_id",
		func(id string) { row.PublicID = id }, row); err != nil {
		if isDuplicateKey(err) {
			return nil, model.ErrDuplicateLLMCall
		}
		return nil, err
	}
	return toLLMCall(&llmCallReadRow{
		Row:             *row,
		TeamPublicID:    canonicalPublicID(stored.TeamID),
		UserPublicID:    optionalCanonicalPublicID(stored.UserID),
		TaskPublicID:    optionalCanonicalPublicID(stored.TaskID),
		TaskRunPublicID: optionalCanonicalPublicID(stored.TaskRunID),
	}), nil
}

// CompleteLLMCall writes the terminal outcome of an open call. Usage is left as
// recorded when the outcome carries none, so an unavailable count is never
// silently written as zero.
func (s *Store) CompleteLLMCall(ctx context.Context, llmCallID string, outcome model.LLMCallOutcome) error {
	updates := map[string]any{
		"status":       outcome.Status,
		"error_class":  outcome.ErrorClass,
		"attempts":     outcome.Attempts,
		"completed_at": outcome.CompletedAt,
	}
	if outcome.UpstreamStartedAt != nil {
		updates["upstream_started_at"] = outcome.UpstreamStartedAt
	}
	if outcome.FirstDeltaAt != nil {
		updates["first_delta_at"] = outcome.FirstDeltaAt
	}
	if usage := outcome.Usage; usage != nil {
		source := usage.Source
		if source == "" {
			source = model.LLMUsageSourceReported
		}
		updates["prompt_tokens"] = usage.PromptTokens
		updates["completion_tokens"] = usage.CompletionTokens
		updates["total_tokens"] = usage.TotalTokens
		updates["cache_read_tokens"] = usage.CacheReadTokens
		updates["cache_write_tokens"] = usage.CacheWriteTokens
		updates["usage_source"] = source
	}
	id, ok := util.CanonicalPublicID(llmCallID)
	if !ok {
		return model.ErrNotFound
	}
	return s.db.WithContext(ctx).Model(&llmCallRow{}).
		Where("public_id = ?", id).
		Updates(updates).Error
}

// GetLLMCall returns one call by ID, or (nil, nil) when not found.
func (s *Store) GetLLMCall(ctx context.Context, llmCallID string) (*model.LLMCall, error) {
	id, ok := util.CanonicalPublicID(llmCallID)
	if !ok {
		return nil, nil
	}
	var row llmCallReadRow
	err := s.llmCallSelect(ctx).Where("llm_call.public_id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toLLMCall(&row), nil
}

// GetLLMCallByClientID returns a team's call by the caller's idempotency key.
// The lookup is team-scoped: one team's key can never resolve another's call.
func (s *Store) GetLLMCallByClientID(ctx context.Context, teamID, clientCallID string) (*model.LLMCall, error) {
	if teamID == "" || clientCallID == "" {
		return nil, nil
	}
	teamKey, err := lookupKey(ctx, s.db, "team", teamID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var row llmCallReadRow
	err = s.llmCallSelect(ctx).
		Where("llm_call.team_id = ? AND llm_call.client_call_id = ?", teamKey, clientCallID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toLLMCall(&row), nil
}

// ListLLMCallsByTaskRun returns one run's calls for a team, oldest first.
//
// Both identifiers are in the WHERE clause. Filtering by run alone and checking
// the team afterwards would still have read another team's row first, and the
// difference matters for a table that records what each team spent.
func (s *Store) ListLLMCallsByTaskRun(ctx context.Context, teamID, taskRunID string) ([]model.LLMCall, error) {
	teamKey, err := lookupKey(ctx, s.db, "team", teamID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	runKey, err := lookupKey(ctx, s.db, "task_run", taskRunID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []llmCallReadRow
	err = s.llmCallSelect(ctx).
		Where("llm_call.team_id = ? AND llm_call.task_run_id = ?", teamKey, runKey).
		Order("llm_call.accepted_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]model.LLMCall, 0, len(rows))
	for i := range rows {
		out = append(out, *toLLMCall(&rows[i]))
	}
	return out, nil
}
