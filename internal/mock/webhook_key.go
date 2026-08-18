package mock

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockUserWebhookKeyStore is an in-memory UserWebhookKeyStore for tests.
// Keys maps a plaintext key to the user that owns it.
type MockUserWebhookKeyStore struct {
	Keys  map[string]string
	Metas map[string][]model.WebhookKeyMeta
	next  int
}

func (m *MockUserWebhookKeyStore) CreateKey(_ context.Context, userID, name string) (string, string, error) {
	if m.Keys == nil {
		m.Keys = make(map[string]string)
	}
	if m.Metas == nil {
		m.Metas = make(map[string][]model.WebhookKeyMeta)
	}
	m.next++
	plaintext := fmt.Sprintf("whsec_mock_%d", m.next)
	keyID := fmt.Sprintf("whk_mock_%d", m.next)
	m.Keys[plaintext] = userID
	m.Metas[userID] = append(m.Metas[userID], model.WebhookKeyMeta{KeyID: keyID, Name: name})
	return plaintext, keyID, nil
}

func (m *MockUserWebhookKeyStore) GetUserIDByKey(_ context.Context, plaintextKey string) (string, error) {
	return m.Keys[plaintextKey], nil
}

func (m *MockUserWebhookKeyStore) ListKeys(_ context.Context, userID string) ([]model.WebhookKeyMeta, error) {
	return m.Metas[userID], nil
}

func (m *MockUserWebhookKeyStore) RevokeKey(_ context.Context, userID, keyID string) error {
	metas := m.Metas[userID]
	for i := range metas {
		if metas[i].KeyID == keyID {
			m.Metas[userID] = append(metas[:i], metas[i+1:]...)
			break
		}
	}
	for plaintext, owner := range m.Keys {
		if owner == userID {
			delete(m.Keys, plaintext)
			break
		}
	}
	return nil
}
