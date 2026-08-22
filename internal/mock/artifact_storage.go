package mock

import (
	"bytes"
	"context"
	"io"
	"sync"

	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
)

// MockArtifactStorage is an in-memory blob.ArtifactStorage for tests.
type MockArtifactStorage struct {
	mu      sync.Mutex
	objects map[string][]byte
	// PutErr, when set, fails every write.
	PutErr error
}

// NewMockArtifactStorage returns a MockArtifactStorage ready for use.
func NewMockArtifactStorage() *MockArtifactStorage {
	return &MockArtifactStorage{objects: make(map[string][]byte)}
}

func mockArtifactKey(ref blob.ArtifactRef) string {
	return blob.ArtifactObjectKey("mock", ref)
}

func (m *MockArtifactStorage) PutArtifact(_ context.Context, ref blob.ArtifactRef, r io.Reader) (string, error) {
	// Read first even when the write is going to fail: the service measures the
	// stream as it passes, and a store that never drains it would make a
	// failure look like an empty file.
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if m.PutErr != nil {
		return "", m.PutErr
	}
	key := mockArtifactKey(ref)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = data
	return key, nil
}

func (m *MockArtifactStorage) OpenArtifact(_ context.Context, ref blob.ArtifactRef) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.objects[mockArtifactKey(ref)]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *MockArtifactStorage) RemoveArtifact(_ context.Context, ref blob.ArtifactRef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, mockArtifactKey(ref))
	return nil
}

// ObjectCount reports how many objects are stored, so a test can prove that a
// failed create left nothing behind.
func (m *MockArtifactStorage) ObjectCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.objects)
}
