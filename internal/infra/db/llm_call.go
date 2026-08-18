package db

import (
	"context"
	"errors"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

// llmCallRow is the managed call ledger. It stores accounting and diagnostic
// metadata only — no prompts, tool payloads, or generated content.
type llmCallRow struct {
	ID           uint    `gorm:"primaryKey;autoIncrement"`
	LLMCallID    string  `gorm:"type:varchar(64);uniqueIndex;not null"`
	ClientCallID *string `gorm:"type:varchar(128);uniqueIndex:idx_llm_call_client,priority:2"`

	// The composite unique index leads with team_id, so team-scoped lookups do
	// not need a second index on this column.
	TeamID    string  `gorm:"type:varchar(64);not null;uniqueIndex:idx_llm_call_client,priority:1"`
	UserID    *string `gorm:"type:varchar(64);index"`
	TaskRunID *string `gorm:"type:varchar(64);index"`

	Surface   string  `gorm:"type:varchar(32)"`
	SessionID *string `gorm:"type:varchar(64)"`
	TaskID    *string `gorm:"type:varchar(64);index"`

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
	UsageSource      string `gorm:"type:varchar(16)"`
}

func (llmCallRow) TableName() string { return "llm_call" }

func toLLMCall(row *llmCallRow) *model.LLMCall {
	if row == nil {
		return nil
	}
	return &model.LLMCall{
		ID:                row.ID,
		LLMCallID:         row.LLMCallID,
		ClientCallID:      row.ClientCallID,
		TeamID:            row.TeamID,
		UserID:            row.UserID,
		TaskRunID:         row.TaskRunID,
		Surface:           row.Surface,
		SessionID:         row.SessionID,
		TaskID:            row.TaskID,
		Alias:             row.Alias,
		TargetID:          row.TargetID,
		ProviderType:      row.ProviderType,
		UpstreamModel:     row.UpstreamModel,
		Streaming:         row.Streaming,
		AcceptedAt:        row.AcceptedAt,
		UpstreamStartedAt: row.UpstreamStartedAt,
		FirstDeltaAt:      row.FirstDeltaAt,
		CompletedAt:       row.CompletedAt,
		Status:            row.Status,
		ErrorClass:        row.ErrorClass,
		Attempts:          row.Attempts,
		PromptTokens:      row.PromptTokens,
		CompletionTokens:  row.CompletionTokens,
		TotalTokens:       row.TotalTokens,
		UsageSource:       row.UsageSource,
	}
}

func toLLMCallRow(call *model.LLMCall) *llmCallRow {
	if call == nil {
		return nil
	}
	return &llmCallRow{
		ID:                call.ID,
		LLMCallID:         call.LLMCallID,
		ClientCallID:      call.ClientCallID,
		TeamID:            call.TeamID,
		UserID:            call.UserID,
		TaskRunID:         call.TaskRunID,
		Surface:           call.Surface,
		SessionID:         call.SessionID,
		TaskID:            call.TaskID,
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
		UsageSource:       call.UsageSource,
	}
}

// OpenLLMCall records an accepted call before the upstream request starts, so a
// call that never returns still leaves evidence that it was attempted.
func (s *Store) OpenLLMCall(ctx context.Context, call *model.LLMCall) (*model.LLMCall, error) {
	if call == nil {
		return nil, errors.New("llm call is required")
	}
	stored := *call
	stored.LLMCallID = util.NewPrefixedID(util.PrefixLLMCall)
	if stored.AcceptedAt == 0 {
		stored.AcceptedAt = time.Now().Unix()
	}
	if stored.Status == "" {
		stored.Status = model.LLMCallStatusAccepted
	}
	if stored.UsageSource == "" {
		stored.UsageSource = model.LLMUsageSourceUnavailable
	}
	row := toLLMCallRow(&stored)
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		if isDuplicateKey(err) {
			return nil, model.ErrDuplicateLLMCall
		}
		return nil, err
	}
	return toLLMCall(row), nil
}

// isDuplicateKey reports whether the error is a unique-constraint violation.
// Checking the constraint rather than looking before inserting is what closes
// the window between two concurrent requests carrying the same client call ID.
func isDuplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysqldriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlDuplicateEntry
}

// mysqlDuplicateEntry is ER_DUP_ENTRY.
const mysqlDuplicateEntry = 1062

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
		updates["usage_source"] = source
	}
	return s.db.WithContext(ctx).Model(&llmCallRow{}).
		Where("llm_call_id = ?", llmCallID).
		Updates(updates).Error
}

// GetLLMCall returns one call by ID, or (nil, nil) when not found.
func (s *Store) GetLLMCall(ctx context.Context, llmCallID string) (*model.LLMCall, error) {
	var row llmCallRow
	err := s.db.WithContext(ctx).Where("llm_call_id = ?", llmCallID).First(&row).Error
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
	var row llmCallRow
	err := s.db.WithContext(ctx).
		Where("team_id = ? AND client_call_id = ?", teamID, clientCallID).
		First(&row).Error
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
	var rows []llmCallRow
	err := s.db.WithContext(ctx).
		Where("team_id = ? AND task_run_id = ?", teamID, taskRunID).
		Order("accepted_at ASC").
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
