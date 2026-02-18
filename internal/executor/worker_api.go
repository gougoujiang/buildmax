package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"buildmax/internal/storage/entity"
)

// getTaskResponse is the JSON response from GET /api/worker/tasks/{task_id} (snake_case).
type getTaskResponse struct {
	TaskID      string  `json:"task_id"`
	WorkspaceID string  `json:"workspace_id"`
	ProjectID   *string `json:"project_id,omitempty"`
	Status      string  `json:"status"`
	Input       string  `json:"input"`
	CreatedBy   string  `json:"created_by"`
	CreatedAt   int64   `json:"created_at"`
}

// GetWorkerTask fetches task details from the server (GET /api/worker/tasks/{task_id}). Returns nil, nil if task not found.
func GetWorkerTask(ctx context.Context, baseURL, token, taskID string, client *http.Client) (*entity.Task, error) {
	url := baseURL + "/api/worker/tasks/" + taskID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("worker API GET %s: %s", url, resp.Status)
	}
	var got getTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		return nil, err
	}
	task := &entity.Task{
		TaskID:      got.TaskID,
		WorkspaceID: got.WorkspaceID,
		ProjectID:   got.ProjectID,
		Status:      got.Status,
		Input:       got.Input,
		CreatedBy:   got.CreatedBy,
		CreatedAt:   got.CreatedAt,
	}
	return task, nil
}

// WorkerHTTPUpdater implements TaskUpdater by calling the server's worker API (PATCH /api/worker/tasks/{task_id}).
type WorkerHTTPUpdater struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// patchBody is the JSON body for PATCH /api/worker/tasks/{task_id} (snake_case).
type patchBody struct {
	Status       string  `json:"status"`
	SessionID    *string `json:"session_id,omitempty"`
	StartedAt    *int64  `json:"started_at,omitempty"`
	EndedAt      *int64  `json:"ended_at,omitempty"`
	Output       *string `json:"output,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
	Artifact     *struct {
		ArtifactID   string `json:"artifact_id"`
		RelativePath string `json:"relative_path"`
	} `json:"artifact,omitempty"`
}

// UpdateTaskStatus sends PATCH to the server to update task status and optional fields.
func (u *WorkerHTTPUpdater) UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errMsg, sessionID *string, artifact *ArtifactPayload) error {
	body := patchBody{Status: status, SessionID: sessionID, StartedAt: startedAt, EndedAt: endedAt, Output: output, ErrorMessage: errMsg}
	if artifact != nil {
		body.Artifact = &struct {
			ArtifactID   string `json:"artifact_id"`
			RelativePath string `json:"relative_path"`
		}{ArtifactID: artifact.ArtifactID, RelativePath: artifact.RelativePath}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := u.BaseURL + "/api/worker/tasks/" + taskID
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if u.Token != "" {
		req.Header.Set("Authorization", "Bearer "+u.Token)
	}
	client := u.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("worker API PATCH %s: %s", url, resp.Status)
	}
	return nil
}
