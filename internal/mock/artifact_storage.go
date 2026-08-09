package mock

import (
	"context"
	"io"

	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
)

// MockArtifactStorage is an in-memory blob.ArtifactStorage for tests.
type MockArtifactStorage struct {
	Results map[string][]byte
	Files   map[string][]byte
}

// NewMockArtifactStorage returns a MockArtifactStorage ready for use.
func NewMockArtifactStorage() *MockArtifactStorage {
	return &MockArtifactStorage{Results: make(map[string][]byte)}
}

func (m *MockArtifactStorage) PutResult(_ context.Context, ref blob.RunRef, data []byte) error {
	m.Results[ref.CreatedBy+"/"+ref.ConversationID+"/"+ref.TaskID+"/"+ref.TaskRunID] = append([]byte(nil), data...)
	return nil
}

func (m *MockArtifactStorage) GetResult(_ context.Context, ref blob.RunRef) ([]byte, error) {
	key := ref.CreatedBy + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID
	if data, ok := m.Results[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}

func (m *MockArtifactStorage) PutArtifactFile(_ context.Context, ref blob.RunObjectRef, r io.Reader) error {
	if m.Files == nil {
		m.Files = make(map[string][]byte)
	}
	key := ref.CreatedBy + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, _ := io.ReadAll(r)
	m.Files[key] = data
	return nil
}

func (m *MockArtifactStorage) GetArtifactFile(_ context.Context, ref blob.RunObjectRef) ([]byte, error) {
	if m.Files == nil {
		return nil, blob.ErrNotFound
	}
	key := ref.CreatedBy + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/" + ref.RelPath
	if data, ok := m.Files[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}
