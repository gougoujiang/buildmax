package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"buildmax/internal/storage/entity"
	"buildmax/internal/workerapi"
)

// ErrTaskAlreadyClaimed is returned when the server responds 409 to PATCH RUNNING (run not SCHEDULED or already RUNNING).
var ErrTaskAlreadyClaimed = errors.New("task run already claimed or not scheduled")

// GetWorkerTaskRun fetches run and task from the server (GET /api/worker/task-runs/{run_id}). Returns nil, nil, nil if not found.
func GetWorkerTaskRun(ctx context.Context, baseURL, token, runID string, client *http.Client) (*entity.TaskRun, *entity.Task, error) {
	url := baseURL + "/api/worker/task-runs/" + runID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("worker API GET %s: %s", url, resp.Status)
	}
	var got workerapi.GetTaskRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		return nil, nil, err
	}
	run := &entity.TaskRun{
		RunID:     got.Run.RunID,
		TaskID:    got.Run.TaskID,
		Input:     got.Run.Input,
		Status:    got.Run.Status,
		CreatedAt: got.Run.CreatedAt,
	}
	task := &entity.Task{
		TaskID:      got.Task.TaskID,
		WorkspaceID: got.Task.WorkspaceID,
		ProjectID:   got.Task.ProjectID,
		SessionID:   got.Task.SessionID,
		LastRunID:   got.Task.LastRunID,
	}
	return run, task, nil
}

// WorkerHTTPUpdater implements TaskRunUpdater by calling the server's worker API (PATCH /api/worker/task-runs/{run_id}).
type WorkerHTTPUpdater struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// UpdateRunStatus sends PATCH to the server to update run status and optional fields.
func (u *WorkerHTTPUpdater) UpdateRunStatus(ctx context.Context, runID, status string, startedAt, endedAt *int64, output, errMsg, sessionID *string, artifact *workerapi.ArtifactPayload) error {
	body := workerapi.PatchTaskRunRequest{
		Status:       status,
		SessionID:    sessionID,
		StartedAt:   startedAt,
		EndedAt:     endedAt,
		Output:      output,
		ErrorMessage: errMsg,
		Artifact:    artifact,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := u.BaseURL + "/api/worker/task-runs/" + runID
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
	if resp.StatusCode == http.StatusConflict {
		return ErrTaskAlreadyClaimed
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("worker API PATCH %s: %s", url, resp.Status)
	}
	return nil
}
