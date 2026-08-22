package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockTaskRunStore is an in-memory TaskRunStore for tests.
type MockTaskRunStore struct {
	Runs     []model.TaskRun
	TaskList []model.Task
	// Artifacts holds the relative paths registered per run, so a test can
	// check that a run's files were kept.
	Artifacts map[string][]string
}

func (m *MockTaskRunStore) CreateTaskRun(_ context.Context, in model.CreateTaskRunInput) (*model.TaskRun, error) {
	// One task holds at most one active run. The real store enforces this, and
	// callers handle the refusal, so a double that quietly allowed a second one
	// would let a test pass on behavior the deployment does not have.
	for i := range m.Runs {
		if m.Runs[i].TaskID == in.TaskID && !model.RunStatusTerminal(m.Runs[i].Status) {
			return nil, model.ErrRunInProgress
		}
	}
	run := model.TaskRun{
		ID:               fmt.Sprintf("r_mock_%d", len(m.Runs)+1),
		TaskID:           in.TaskID,
		Input:            in.Input,
		CreatedBy:        in.CreatedBy,
		CreatedByType:    in.CreatedByType,
		TriggerSource:    in.TriggerSource,
		Status:           string(model.RunStatusPending),
		RetryOfTaskRunID: in.RetryOfTaskRunID,
		CreatedAt:        time.Now().Unix(),
	}
	m.Runs = append(m.Runs, run)
	return &m.Runs[len(m.Runs)-1], nil
}
func (m *MockTaskRunStore) CountTaskRunsByStatus(_ context.Context) (map[string]int, error) {
	out := make(map[string]int)
	for _, run := range m.Runs {
		out[run.Status]++
	}
	return out, nil
}

func (m *MockTaskRunStore) GetNextPendingTaskRun(_ context.Context) (*model.TaskRun, error) {
	return nil, nil
}
func (m *MockTaskRunStore) GetTaskRun(_ context.Context, taskRunID string) (*model.TaskRun, error) {
	for i := range m.Runs {
		if m.Runs[i].ID == taskRunID {
			return &m.Runs[i], nil
		}
	}
	return nil, nil
}
func (m *MockTaskRunStore) GetTaskRunWithTask(_ context.Context, taskRunID string) (*model.TaskRun, *model.Task, error) {
	var run *model.TaskRun
	for i := range m.Runs {
		if m.Runs[i].ID == taskRunID {
			run = &m.Runs[i]
			break
		}
	}
	if run == nil {
		return nil, nil, nil
	}
	var task *model.Task
	for i := range m.TaskList {
		if m.TaskList[i].ID == run.TaskID {
			task = &m.TaskList[i]
			break
		}
	}
	return run, task, nil
}
func (m *MockTaskRunStore) ListTaskRunIDsByTasks(_ context.Context, taskIDs []string) (map[string][]string, error) {
	want := make(map[string]bool, len(taskIDs))
	for _, id := range taskIDs {
		want[id] = true
	}
	out := make(map[string][]string)
	for i := len(m.Runs) - 1; i >= 0; i-- {
		if want[m.Runs[i].TaskID] {
			out[m.Runs[i].TaskID] = append(out[m.Runs[i].TaskID], m.Runs[i].ID)
		}
	}
	return out, nil
}

func (m *MockTaskRunStore) GetActiveTaskRunByTask(_ context.Context, taskID string) (*model.TaskRun, error) {
	for i := range m.Runs {
		if m.Runs[i].TaskID != taskID {
			continue
		}
		if !model.RunStatusTerminal(m.Runs[i].Status) {
			return &m.Runs[i], nil
		}
	}
	return nil, nil
}

func (m *MockTaskRunStore) RequestTaskRunCancel(_ context.Context, taskRunID, requestedBy string, requestedAt int64) (bool, error) {
	for i := range m.Runs {
		if m.Runs[i].ID != taskRunID {
			continue
		}
		if model.RunStatusTerminal(m.Runs[i].Status) || m.Runs[i].CancelRequestedAt != nil {
			return false, nil
		}
		m.Runs[i].CancelRequestedAt = &requestedAt
		m.Runs[i].CancelRequestedBy = &requestedBy
		return true, nil
	}
	return false, nil
}

func (m *MockTaskRunStore) ClaimTaskRun(ctx context.Context, in model.ClaimTaskRunInput) (bool, error) {
	for i := range m.Runs {
		if m.Runs[i].ID == in.TaskRunID && m.Runs[i].Status == string(in.ExpectedStatus) {
			m.Runs[i].Status = string(in.NewStatus)
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
			return true, nil
		}
	}
	return false, nil
}
func (m *MockTaskRunStore) UpdateRun(ctx context.Context, in model.UpdateTaskRunInput) error {
	for i := range m.Runs {
		if m.Runs[i].ID == in.TaskRunID {
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

// OnRunComplete records the run's artifacts and copies its terminal state onto
// its task, which is what the real store does for every terminal status that
// leaves files behind.
func (m *MockTaskRunStore) OnRunComplete(ctx context.Context, taskRunID string, relativePaths []string) error {
	if m.Artifacts == nil {
		m.Artifacts = make(map[string][]string)
	}
	m.Artifacts[taskRunID] = append(m.Artifacts[taskRunID], relativePaths...)
	return m.SyncTaskFromRun(ctx, taskRunID)
}

// SyncTaskFromRun copies the run's terminal state onto its task, which is what
// the real store does and what callers assert on after a cancel or a failure.
func (m *MockTaskRunStore) SyncTaskFromRun(_ context.Context, taskRunID string) error {
	for i := range m.Runs {
		if m.Runs[i].ID != taskRunID {
			continue
		}
		run := m.Runs[i]
		for j := range m.TaskList {
			if m.TaskList[j].ID != run.TaskID {
				continue
			}
			m.TaskList[j].Status = run.Status
			m.TaskList[j].LastRunID = &run.ID
			m.TaskList[j].Output = run.Output
			m.TaskList[j].StartedAt = run.StartedAt
			m.TaskList[j].EndedAt = run.EndedAt
			m.TaskList[j].ErrorMessage = run.ErrorMessage
		}
		return nil
	}
	return nil
}
