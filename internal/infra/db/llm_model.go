package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

// llmModelRow is the managed model catalog.
//
// APIKey is the one column that must never be selected by a general read. The
// store exposes it through LLMModelCredential alone, so a query that forgets to
// exclude it cannot exist.
type llmModelRow struct {
	ID            uint   `gorm:"primaryKey;autoIncrement"`
	LLMModelID    string `gorm:"type:varchar(64);uniqueIndex;not null"`
	Name          string `gorm:"type:varchar(128);uniqueIndex;not null"`
	ProviderType  string `gorm:"type:varchar(32);not null"`
	APIURL        string `gorm:"type:varchar(512);not null"`
	APIKey        string `gorm:"type:varchar(512);not null"`
	Model         string `gorm:"type:varchar(128);not null"`
	ContextWindow int    `gorm:"not null;default:0"`
	CallTimeout   int    `gorm:"not null;default:0"`
	// Capabilities is a comma-separated list. The set is small, closed, and only
	// ever read whole, so a join table would buy nothing.
	Capabilities string `gorm:"type:varchar(255)"`
	Enabled      bool   `gorm:"not null;default:true"`
	CreatedAt    int64  `gorm:"autoCreateTime;index"`
	UpdatedAt    int64  `gorm:"autoUpdateTime"`
}

func (llmModelRow) TableName() string { return "llm_model" }

func toLLMModel(row *llmModelRow) *model.LLMModel {
	if row == nil {
		return nil
	}
	return &model.LLMModel{
		ID:            row.ID,
		LLMModelID:    row.LLMModelID,
		Name:          row.Name,
		ProviderType:  row.ProviderType,
		APIURL:        row.APIURL,
		Model:         row.Model,
		ContextWindow: row.ContextWindow,
		CallTimeout:   row.CallTimeout,
		Capabilities:  splitCapabilities(row.Capabilities),
		Enabled:       row.Enabled,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func splitCapabilities(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func joinCapabilities(in []string) string {
	out := make([]string, 0, len(in))
	for _, c := range in {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return strings.Join(out, ",")
}

// llmModelColumns is every column except the credential. Reads name it
// explicitly so adding a column later cannot silently start returning the key.
var llmModelColumns = []string{
	"id", "llm_model_id", "name", "provider_type", "api_url", "model",
	"context_window", "call_timeout", "capabilities", "enabled",
	"created_at", "updated_at",
}

// CreateLLMModel stores a new model. The name is unique so an operator cannot
// end up with two catalog entries that look identical in a listing.
func (s *Store) CreateLLMModel(ctx context.Context, in model.CreateLLMModelInput) (*model.LLMModel, error) {
	now := time.Now().Unix()
	row := &llmModelRow{
		LLMModelID:    util.NewPrefixedID(util.PrefixLLMModel),
		Name:          in.Name,
		ProviderType:  in.ProviderType,
		APIURL:        in.APIURL,
		APIKey:        in.APIKey,
		Model:         in.Model,
		ContextWindow: in.ContextWindow,
		CallTimeout:   in.CallTimeout,
		Capabilities:  joinCapabilities(in.Capabilities),
		Enabled:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		if isDuplicateKey(err) {
			return nil, model.ErrLLMModelNameTaken
		}
		return nil, err
	}
	return toLLMModel(row), nil
}

// GetLLMModel returns one model by ID, or (nil, nil) when not found.
func (s *Store) GetLLMModel(ctx context.Context, llmModelID string) (*model.LLMModel, error) {
	if llmModelID == "" {
		return nil, nil
	}
	var row llmModelRow
	err := s.db.WithContext(ctx).Select(llmModelColumns).
		Where("llm_model_id = ?", llmModelID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toLLMModel(&row), nil
}

// ListLLMModels returns every model, enabled or not, oldest first.
func (s *Store) ListLLMModels(ctx context.Context) ([]model.LLMModel, error) {
	var rows []llmModelRow
	if err := s.db.WithContext(ctx).Select(llmModelColumns).
		Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]model.LLMModel, 0, len(rows))
	for i := range rows {
		out = append(out, *toLLMModel(&rows[i]))
	}
	return out, nil
}

// SetLLMModelEnabled retires or restores a model.
func (s *Store) SetLLMModelEnabled(ctx context.Context, llmModelID string, enabled bool) error {
	res := s.db.WithContext(ctx).Model(&llmModelRow{}).
		Where("llm_model_id = ?", llmModelID).
		Updates(map[string]any{"enabled": enabled, "updated_at": time.Now().Unix()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("model not found")
	}
	return nil
}

// LLMModelCredential returns the upstream key for a model.
//
// This is the only read that touches the credential column, which is what makes
// "the key reaches the provider client and nothing else" checkable rather than
// a matter of care.
func (s *Store) LLMModelCredential(ctx context.Context, llmModelID string) (string, error) {
	if llmModelID == "" {
		return "", errors.New("model id is required")
	}
	var row llmModelRow
	err := s.db.WithContext(ctx).Select("api_key").
		Where("llm_model_id = ?", llmModelID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", errors.New("model not found")
	}
	if err != nil {
		return "", err
	}
	return row.APIKey, nil
}
