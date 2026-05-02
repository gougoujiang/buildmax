package mock

import (
	"context"

	"buildmax/internal/infra/db"
)

// MockRunOutputLister is an in-memory RunOutputLister for tests.
type MockRunOutputLister struct {
	List        []db.ArtifactWithTask
	ListErr     error
	OutputFiles map[string][]db.TaskRunArtifact
}

func (m *MockRunOutputLister) ListRunOutputsByConversation(_ context.Context, conversationID string, taskID *string) ([]db.ArtifactWithTask, error) {
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	return m.List, nil
}

func (m *MockRunOutputLister) GetTaskRunOutputFiles(_ context.Context, chatRunID string) ([]db.TaskRunArtifact, error) {
	if m.OutputFiles != nil {
		return m.OutputFiles[chatRunID], nil
	}
	return nil, nil
}
