package mock

import (
	"context"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
)

// MockRunOutputLister is an in-memory RunOutputLister for tests.
type MockRunOutputLister struct {
	List        []coretask.RunOutputListing
	ListErr     error
	OutputFiles map[string][]coretask.RunOutputFile
}

func (m *MockRunOutputLister) ListRunOutputsByConversation(_ context.Context, conversationID string, taskID *string) ([]coretask.RunOutputListing, error) {
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	return m.List, nil
}

func (m *MockRunOutputLister) GetTaskRunOutputFiles(_ context.Context, taskRunID string) ([]coretask.RunOutputFile, error) {
	if m.OutputFiles != nil {
		return m.OutputFiles[taskRunID], nil
	}
	return nil, nil
}
