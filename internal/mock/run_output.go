package mock

import (
	"context"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockRunOutputLister is an in-memory RunOutputLister for tests.
type MockRunOutputLister struct {
	List        []model.ArtifactWithTask
	ListErr     error
	OutputFiles map[string][]model.TaskRunArtifact
}

func (m *MockRunOutputLister) ListRunOutputsByConversation(_ context.Context, conversationID string, taskID *string) ([]model.ArtifactWithTask, error) {
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	return m.List, nil
}

func (m *MockRunOutputLister) GetTaskRunOutputFiles(_ context.Context, taskRunID string) ([]model.TaskRunArtifact, error) {
	if m.OutputFiles != nil {
		return m.OutputFiles[taskRunID], nil
	}
	return nil, nil
}
