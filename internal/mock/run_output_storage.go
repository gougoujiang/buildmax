package mock

import (
	"context"
	"io"

	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
)

// MockRunOutputStorage is an in-memory blob.RunOutputStorage for tests.
type MockRunOutputStorage struct {
	Results map[string][]byte
	Files   map[string][]byte
}

// NewMockRunOutputStorage returns a MockRunOutputStorage ready for use.
func NewMockRunOutputStorage() *MockRunOutputStorage {
	return &MockRunOutputStorage{Results: make(map[string][]byte)}
}

func (m *MockRunOutputStorage) PutResult(_ context.Context, ref blob.RunRef, data []byte) error {
	m.Results[ref.CreatedBy+"/"+ref.ConversationID+"/"+ref.TaskID+"/"+ref.TaskRunID] = append([]byte(nil), data...)
	return nil
}

func (m *MockRunOutputStorage) GetResult(_ context.Context, ref blob.RunRef) ([]byte, error) {
	key := ref.CreatedBy + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID
	if data, ok := m.Results[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}

func (m *MockRunOutputStorage) PutRunOutputFile(_ context.Context, ref blob.RunObjectRef, r io.Reader) error {
	if m.Files == nil {
		m.Files = make(map[string][]byte)
	}
	key := ref.CreatedBy + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, _ := io.ReadAll(r)
	m.Files[key] = data
	return nil
}

func (m *MockRunOutputStorage) GetRunOutputFile(_ context.Context, ref blob.RunObjectRef) ([]byte, error) {
	if m.Files == nil {
		return nil, blob.ErrNotFound
	}
	key := ref.CreatedBy + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/" + ref.RelPath
	if data, ok := m.Files[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}
