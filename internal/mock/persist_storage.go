package mock

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"io"

	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
)

// MockPersistStorage is an in-memory blob.PersistStorage for tests. Only the
// run-global half is backed by a map; the home-file methods satisfy the
// interface and are inert, because the handlers that need this mock read run
// state, not team files.
type MockPersistStorage struct {
	// RunGlobal is keyed by teamID/taskID/taskRunID/relPath.
	RunGlobal map[string][]byte
	// RunArtifacts uses the same key shape.
	RunArtifacts map[string][]byte
}

// NewMockPersistStorage returns a MockPersistStorage ready for use.
func NewMockPersistStorage() *MockPersistStorage {
	return &MockPersistStorage{
		RunGlobal:    make(map[string][]byte),
		RunArtifacts: make(map[string][]byte),
	}
}

func runObjectKey(ref blob.RunObjectRef) string {
	return ref.TeamID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/" + ref.RelPath
}

func (m *MockPersistStorage) PutRunGlobal(_ context.Context, ref blob.RunObjectRef, r io.Reader) error {
	if m.RunGlobal == nil {
		m.RunGlobal = make(map[string][]byte)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.RunGlobal[runObjectKey(ref)] = data
	return nil
}

func (m *MockPersistStorage) GetRunGlobal(_ context.Context, ref blob.RunObjectRef) ([]byte, error) {
	if data, ok := m.RunGlobal[runObjectKey(ref)]; ok {
		return data, nil
	}
	return nil, apierr.ErrNotFound
}

func (m *MockPersistStorage) PutRunArtifacts(_ context.Context, ref blob.RunObjectRef, r io.Reader) error {
	if m.RunArtifacts == nil {
		m.RunArtifacts = make(map[string][]byte)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.RunArtifacts[runObjectKey(ref)] = data
	return nil
}

func (m *MockPersistStorage) GetRunArtifacts(_ context.Context, ref blob.RunObjectRef) ([]byte, error) {
	if data, ok := m.RunArtifacts[runObjectKey(ref)]; ok {
		return data, nil
	}
	return nil, apierr.ErrNotFound
}

func (m *MockPersistStorage) Put(_ context.Context, _ string, _ string, _ io.Reader) error {
	return nil
}

func (m *MockPersistStorage) Get(_ context.Context, _ string, _ string) ([]byte, error) {
	return nil, apierr.ErrNotFound
}

func (m *MockPersistStorage) ListFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (m *MockPersistStorage) MaterializeToDir(_ context.Context, _ string, _ string) error {
	return nil
}
