package model

import (
	"context"
	"errors"
)

// ErrLLMModelNameTaken is returned when an operator reuses a model name.
var ErrLLMModelNameTaken = errors.New("a model with this name already exists")

// LLMModel is one operator-approved upstream the managed gateway may call.
//
// The record deliberately has no credential field. The key lives in the same
// table but is read only by the component that opens a provider connection, so
// listing, resolving, and diagnosing models can never carry it by accident.
// See docs/design/llm-gateway.md.
type LLMModel struct {
	ID         uint   `json:"-"`
	LLMModelID string `json:"llm_model_id"`
	// Name is the operator-facing name, unique within a deployment.
	Name string `json:"name"`
	// ProviderType selects the client implementation.
	ProviderType string `json:"provider_type"`
	// APIURL is the upstream base URL.
	APIURL string `json:"api_url"`
	// Model is the provider's own model identifier.
	Model string `json:"model"`
	// ContextWindow is the usable context size; 0 uses the client default.
	ContextWindow int `json:"context_window,omitempty"`
	// CallTimeout bounds one upstream call in seconds; 0 uses the client default.
	CallTimeout int `json:"call_timeout,omitempty"`
	// MaxTokens caps one response; 0 uses the client default. The Anthropic
	// protocol requires the field, so a target speaking it always sends one.
	MaxTokens int `json:"max_tokens,omitempty"`
	// Capabilities is what this model supports, e.g. "text_chat".
	Capabilities []string `json:"capabilities,omitempty"`
	// Enabled lets an operator retire a model without deleting it.
	Enabled   bool  `json:"enabled"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// CreateLLMModelInput is a new catalog row, including the credential that the
// record itself never carries afterwards.
type CreateLLMModelInput struct {
	Name          string
	ProviderType  string
	APIURL        string
	APIKey        string
	Model         string
	ContextWindow int
	CallTimeout   int
	MaxTokens     int
	Capabilities  []string
}

// LLMModelStore persists the managed model catalog.
//
// Reading a model and reading its credential are separate operations on
// purpose: everything that lists, resolves, or reports a model uses the first,
// and only the client factory uses the second.
type LLMModelStore interface {
	// CreateLLMModel stores a new model and returns it without its credential.
	CreateLLMModel(ctx context.Context, in CreateLLMModelInput) (*LLMModel, error)
	// GetLLMModel returns one model by ID, or (nil, nil) when not found.
	GetLLMModel(ctx context.Context, llmModelID string) (*LLMModel, error)
	// ListLLMModels returns every model, enabled or not, oldest first.
	ListLLMModels(ctx context.Context) ([]LLMModel, error)
	// SetLLMModelEnabled retires or restores a model.
	SetLLMModelEnabled(ctx context.Context, llmModelID string, enabled bool) error
	// LLMModelCredential returns the upstream key for a model. It is the only
	// way a credential leaves the store.
	LLMModelCredential(ctx context.Context, llmModelID string) (string, error)
}
