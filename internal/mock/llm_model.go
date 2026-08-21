package mock

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockLLMModelStore is an in-memory LLMModelStore for tests.
//
// Credentials are kept in a separate map, mirroring the real store: reading a
// model and reading its key are different operations, and a mock that stapled
// the key onto the record would let a handler test pass while the endpoint
// serialized it.
type MockLLMModelStore struct {
	Models      []model.LLMModel
	Credentials map[string]string
	Err         error
	next        int
}

func (m *MockLLMModelStore) CreateLLMModel(_ context.Context, in model.CreateLLMModelInput) (*model.LLMModel, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	for _, existing := range m.Models {
		if existing.Name == in.Name {
			return nil, model.ErrLLMModelNameTaken
		}
	}
	m.next++
	created := model.LLMModel{
		LLMModelID:    fmt.Sprintf("lm_mock_%d", m.next),
		Name:          in.Name,
		ProviderType:  in.ProviderType,
		APIURL:        in.APIURL,
		Model:         in.Model,
		ContextWindow: in.ContextWindow,
		CallTimeout:   in.CallTimeout,
		MaxTokens:     in.MaxTokens,
		Reasoning:     in.Reasoning,
		PromptCache:   in.PromptCache,
		Vision:        in.Vision,
		Capabilities:  in.Capabilities,
		Enabled:       true,
	}
	m.Models = append(m.Models, created)
	if m.Credentials == nil {
		m.Credentials = make(map[string]string)
	}
	m.Credentials[created.LLMModelID] = in.APIKey
	return &created, nil
}

func (m *MockLLMModelStore) GetLLMModel(_ context.Context, llmModelID string) (*model.LLMModel, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	for i := range m.Models {
		if m.Models[i].LLMModelID == llmModelID {
			found := m.Models[i]
			return &found, nil
		}
	}
	return nil, nil
}

func (m *MockLLMModelStore) ListLLMModels(_ context.Context) ([]model.LLMModel, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return append([]model.LLMModel(nil), m.Models...), nil
}

func (m *MockLLMModelStore) SetLLMModelEnabled(_ context.Context, llmModelID string, enabled bool) error {
	if m.Err != nil {
		return m.Err
	}
	for i := range m.Models {
		if m.Models[i].LLMModelID == llmModelID {
			m.Models[i].Enabled = enabled
			return nil
		}
	}
	return nil
}

func (m *MockLLMModelStore) LLMModelCredential(_ context.Context, llmModelID string) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	return m.Credentials[llmModelID], nil
}
