package mock

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockTaskStore is an in-memory TaskStore for tests.
type MockTaskStore struct {
	List      []model.Task
	ListErr   error
	Create    *model.Task
	CreateErr error
	// Created records what each CreateTask was asked for. The returned Task
	// drops most of it, so provenance a caller set can only be asserted here.
	Created []model.CreateTaskInput
}

func (m *MockTaskStore) ListTasksByConversation(_ context.Context, conversationID string, order string) ([]model.Task, error) {
	list, _, err := m.ListTasksByConversationPaginated(context.Background(), conversationID, false, 0, 0)
	if err != nil {
		return nil, err
	}
	if order == "asc" {
		for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
			list[i], list[j] = list[j], list[i]
		}
	}
	return list, nil
}

func (m *MockTaskStore) ListTasksByConversationPaginated(_ context.Context, conversationID string, executedOnly bool, limit, offset int) ([]model.Task, int, error) {
	if m.ListErr != nil {
		return nil, 0, m.ListErr
	}
	var filtered []model.Task
	for _, c := range m.List {
		if c.ConversationID != conversationID {
			continue
		}
		if executedOnly && (c.LastRunID == nil || *c.LastRunID == "") {
			continue
		}
		filtered = append(filtered, c)
	}
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []model.Task{}, total, nil
	}
	end := offset + limit
	if limit <= 0 {
		end = len(filtered)
	}
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *MockTaskStore) ListTasksByIssue(_ context.Context, issueID string, limit, offset int) ([]model.Task, int, error) {
	if m.ListErr != nil {
		return nil, 0, m.ListErr
	}
	var filtered []model.Task
	for _, task := range m.List {
		if task.IssueID != nil && *task.IssueID == issueID {
			filtered = append(filtered, task)
		}
	}
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []model.Task{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *MockTaskStore) CreateTask(_ context.Context, in *model.CreateTaskInput) (*model.Task, error) {
	if m.CreateErr != nil {
		return nil, m.CreateErr
	}
	if m.Create != nil {
		return m.Create, nil
	}
	if in == nil {
		return nil, nil
	}
	m.Created = append(m.Created, *in)
	id := len(m.List) + 1
	taskID := fmt.Sprintf("t_mock_%d", id)
	lastRunID := fmt.Sprintf("r_mock_%d", id)
	task := &model.Task{
		ID:             taskID,
		ConversationID: in.ConversationID,
		TeamID:         in.TeamID,
		Status:         "PENDING",
		Input:          in.Input,
		Title:          in.Title,
		CreatedBy:      in.CreatedBy,
		CreatedAt:      seqTime(12345),
		AgentID:        in.AgentID,
		IssueID:        in.IssueID,
	}
	task.LastRunID = &lastRunID
	m.List = append(m.List, *task)
	return task, nil
}

func (m *MockTaskStore) GetTaskBySessionID(_ context.Context, sessionID string) (*model.Task, error) {
	for i := range m.List {
		if m.List[i].SessionID != nil && *m.List[i].SessionID == sessionID {
			return &m.List[i], nil
		}
	}
	return nil, nil
}

func (m *MockTaskStore) UpdateTask(_ context.Context, in model.UpdateTaskInput) error {
	for i := range m.List {
		if m.List[i].ID == in.TaskID {
			m.List[i].Status = in.Status
			if in.StartedAt != nil {
				m.List[i].StartedAt = in.StartedAt
			}
			if in.EndedAt != nil {
				m.List[i].EndedAt = in.EndedAt
			}
			if in.Output != nil {
				m.List[i].Output = in.Output
			}
			if in.ErrorMessage != nil {
				m.List[i].ErrorMessage = in.ErrorMessage
			}
			if in.SessionID != nil {
				m.List[i].SessionID = in.SessionID
			}
			return nil
		}
	}
	return nil
}

func (m *MockTaskStore) ClaimTask(_ context.Context, in model.ClaimTaskInput) (bool, error) {
	for i := range m.List {
		if m.List[i].ID == in.TaskID && m.List[i].Status == in.ExpectedStatus {
			m.List[i].Status = in.NewStatus
			if in.StartedAt != nil {
				m.List[i].StartedAt = in.StartedAt
			}
			if in.EndedAt != nil {
				m.List[i].EndedAt = in.EndedAt
			}
			if in.Output != nil {
				m.List[i].Output = in.Output
			}
			if in.ErrorMessage != nil {
				m.List[i].ErrorMessage = in.ErrorMessage
			}
			if in.SessionID != nil {
				m.List[i].SessionID = in.SessionID
			}
			return true, nil
		}
	}
	return false, nil
}

func (m *MockTaskStore) GetTask(_ context.Context, taskID string) (*model.Task, error) {
	for i := range m.List {
		if m.List[i].ID == taskID {
			return &m.List[i], nil
		}
	}
	return nil, nil
}
