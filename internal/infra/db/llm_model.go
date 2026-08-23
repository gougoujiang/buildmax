package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// llmModelRow is the managed model catalog.
//
// APIKey is the one column that must never be selected by a general read. The
// store exposes it through LLMModelCredential alone, so a query that forgets to
// exclude it cannot exist.
type llmModelRow struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID      string `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_llm_model_public_id;not null"`
	Name          string `gorm:"type:varchar(128);uniqueIndex;not null"`
	ProviderType  string `gorm:"type:varchar(32);not null"`
	APIURL        string `gorm:"type:varchar(512);not null"`
	APIKey        string `gorm:"type:varchar(512);not null"`
	Model         string `gorm:"type:varchar(128);not null"`
	ContextWindow int    `gorm:"not null;default:0"`
	CallTimeout   int    `gorm:"not null;default:0"`
	MaxTokens     int    `gorm:"not null;default:0"`
	Reasoning     string `gorm:"type:varchar(16);not null;default:''"`
	PromptCache   bool   `gorm:"not null;default:false"`
	// CacheMode and CacheTTL are the structured prompt-cache policy that
	// supersedes PromptCache. Empty means unset, which resolves from the older
	// column; the two are kept side by side because a bool cannot record the
	// difference between "cache off" and "nobody chose".
	CacheMode string `gorm:"size:16;not null;default:''"`
	CacheTTL  string `gorm:"size:16;not null;default:''"`
	Vision    bool   `gorm:"not null;default:false"`
	// Capabilities is a comma-separated list. The set is small, closed, and only
	// ever read whole, so a join table would buy nothing.
	Capabilities string    `gorm:"type:varchar(255)"`
	Enabled      bool      `gorm:"not null;default:true"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}

func (llmModelRow) TableName() string { return "llm_model" }

func toLLMModel(row *llmModelRow) *model.LLMModel {
	if row == nil {
		return nil
	}
	return &model.LLMModel{
		ID:            row.PublicID,
		Name:          row.Name,
		ProviderType:  row.ProviderType,
		APIURL:        row.APIURL,
		Model:         row.Model,
		ContextWindow: row.ContextWindow,
		CallTimeout:   row.CallTimeout,
		MaxTokens:     row.MaxTokens,
		Reasoning:     row.Reasoning,
		PromptCache:   row.PromptCache,
		CacheMode:     row.CacheMode,
		CacheTTL:      row.CacheTTL,
		Vision:        row.Vision,
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
	"id", "public_id", "name", "provider_type", "api_url", "model",
	"context_window", "call_timeout", "max_tokens", "reasoning", "prompt_cache",
	"vision", "capabilities", "enabled",
	"created_at", "updated_at",
}

// CreateLLMModel stores a new model. The name is unique so an operator cannot
// end up with two catalog entries that look identical in a listing.
func (s *Store) CreateLLMModel(ctx context.Context, in model.CreateLLMModelInput) (*model.LLMModel, error) {
	now := time.Now().UTC()
	row := &llmModelRow{
		Name:          in.Name,
		ProviderType:  in.ProviderType,
		APIURL:        in.APIURL,
		APIKey:        in.APIKey,
		Model:         in.Model,
		ContextWindow: in.ContextWindow,
		CallTimeout:   in.CallTimeout,
		MaxTokens:     in.MaxTokens,
		Reasoning:     in.Reasoning,
		PromptCache:   in.PromptCache,
		CacheMode:     in.CacheMode,
		CacheTTL:      in.CacheTTL,
		Vision:        in.Vision,
		Capabilities:  joinCapabilities(in.Capabilities),
		Enabled:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := createWithPublicID(ctx, s.db, "uq_llm_model_public_id",
		func(id string) { row.PublicID = id }, row); err != nil {
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
		Where("public_id = ?", canonicalPublicID(llmModelID)).First(&row).Error
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
		Where("public_id = ?", canonicalPublicID(llmModelID)).
		Updates(map[string]any{"enabled": enabled, "updated_at": time.Now().UTC()})
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
		Where("public_id = ?", canonicalPublicID(llmModelID)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", errors.New("model not found")
	}
	if err != nil {
		return "", err
	}
	return row.APIKey, nil
}
