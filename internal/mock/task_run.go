package mock

import (
	"context"
	"fmt"
	"time"

	"buildmax/internal/infra/db"
)

// MockTaskRunStore is an in-memory TaskRunStore for tests.
type MockTaskRunStore struct {
	Runs     []db.TaskRun
	TaskList []db.Task
}

func (m *MockTaskRunStore) CreateTaskRun(_ context.Context, taskID, input, createdBy, createdByType, triggerSource string) (*db.TaskRun, error) {
	run := db.TaskRun{
		TaskRunID:     fmt.Sprintf("r_mock_%d", len(m.Runs)+1),
		TaskID:        taskID,
		Input:         input,
		CreatedBy:     createdBy,
		CreatedByType: createdByType,
		TriggerSource: triggerSource,
		Status:        string(db.RunStatusPending),
		CreatedAt:     time.Now().Unix(),
	}
	m.Runs = append(m.Runs, run)
	return &m.Runs[len(m.Runs)-1], nil
}
func (m *MockTaskRunStore) GetNextPendingTaskRun(_ context.Context) (*db.TaskRun, error) {
	return nil, nil
}
func (m *MockTaskRunStore) GetTaskRun(_ context.Context, taskRunID string) (*db.TaskRun, error) {
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == taskRunID {
			return &m.Runs[i], nil
		}
	}
	return nil, nil
}
func (m *MockTaskRunStore) GetTaskRunWithTask(_ context.Context, taskRunID string) (*db.TaskRun, *db.Task, error) {
	var run *db.TaskRun
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == taskRunID {
			run = &m.Runs[i]
			break
		}
	}
	if run == nil {
		return nil, nil, nil
	}
	var task *db.Task
	for i := range m.TaskList {
		if m.TaskList[i].TaskID == run.TaskID {
			task = &m.TaskList[i]
			break
		}
	}
	return run, task, nil
}
func (m *MockTaskRunStore) ClaimTaskRun(ctx context.Context, in db.ClaimTaskRunInput) (bool, error) {
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == in.TaskRunID && m.Runs[i].Status == string(in.ExpectedStatus) {
			m.Runs[i].Status = string(in.NewStatus)
			if in.StartedAt != nil {
				m.Runs[i].StartedAt = in.StartedAt
			}
			if in.SessionID != nil {
				m.Runs[i].SessionID = in.SessionID
			}
			return true, nil
		}
	}
	return false, nil
}
func (m *MockTaskRunStore) UpdateRun(ctx context.Context, in db.UpdateTaskRunInput) error {
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == in.TaskRunID {
			m.Runs[i].Status = string(in.Status)
			if in.StartedAt != nil {
				m.Runs[i].StartedAt = in.StartedAt
			}
			if in.EndedAt != nil {
				m.Runs[i].EndedAt = in.EndedAt
			}
			if in.Output != nil {
				m.Runs[i].Output = in.Output
			}
			if in.ErrorMessage != nil {
				m.Runs[i].ErrorMessage = in.ErrorMessage
			}
			if in.SessionID != nil {
				m.Runs[i].SessionID = in.SessionID
			}
			if in.PromptTokens != nil {
				m.Runs[i].PromptTokens = in.PromptTokens
			}
			if in.CompletionTokens != nil {
				m.Runs[i].CompletionTokens = in.CompletionTokens
			}
			return nil
		}
	}
	return nil
}
func (m *MockTaskRunStore) UpdateTaskRunWorkerInfo(_ context.Context, taskRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error {
	return nil
}
func (m *MockTaskRunStore) OnRunComplete(_ context.Context, taskRunID string, relativePaths []string) error {
	return nil
}
func (m *MockTaskRunStore) SyncTaskFromRun(_ context.Context, taskRunID string) error {
	return nil
}
