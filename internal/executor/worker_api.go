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

// StreamSender sends run output deltas to the server (e.g. for live streaming). Optional; when nil, run output is not streamed.
type StreamSender interface {
	SendDelta(ctx context.Context, chatRunID, delta string) error
}

// ErrChatRunAlreadyClaimed is returned when the server responds 409 to PATCH RUNNING (run not SCHEDULED or already RUNNING).
var ErrChatRunAlreadyClaimed = errors.New("chat run already claimed or not scheduled")

// GetWorkerChatRun fetches run and chat from the server (GET /api/worker/chat-runs/{chat_run_id}). Returns nil, nil, nil if not found.
func GetWorkerChatRun(ctx context.Context, baseURL, token, chatRunID string, client *http.Client) (*entity.ChatRun, *entity.Chat, error) {
	url := baseURL + "/api/worker/chat-runs/" + chatRunID
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
	var got workerapi.GetChatRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		return nil, nil, err
	}
	run := &entity.ChatRun{
		ChatRunID: got.Run.ChatRunID,
		ChatID:    got.Run.ChatID,
		Input:     got.Run.Input,
		Status:    got.Run.Status,
		CreatedAt: got.Run.CreatedAt,
	}
	chat := &entity.Chat{
		ChatID:      got.Chat.ChatID,
		WorkspaceID: got.Chat.WorkspaceID,
		SessionID:   got.Chat.SessionID,
		LastRunID:   got.Chat.LastRunID,
	}
	return run, chat, nil
}

// WorkerHTTPUpdater implements ChatRunUpdater by calling the server's worker API (PATCH /api/worker/chat-runs/{chat_run_id}).
type WorkerHTTPUpdater struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// UpdateRunStatus sends PATCH to the server to update run status and optional fields.
func (u *WorkerHTTPUpdater) UpdateRunStatus(ctx context.Context, chatRunID, status string, startedAt, endedAt *int64, output, errMsg, sessionID *string, artifact *workerapi.ArtifactPayload) error {
	body := workerapi.PatchChatRunRequest{
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
	url := u.BaseURL + "/api/worker/chat-runs/" + chatRunID
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
		return ErrChatRunAlreadyClaimed
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("worker API PATCH %s: %s", url, resp.Status)
	}
	return nil
}

// WorkerHTTPStreamSender implements StreamSender by POSTing each delta to the server's worker stream endpoint.
type WorkerHTTPStreamSender struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// SendDelta POSTs the delta to POST /api/worker/chat-runs/{chat_run_id}/stream.
func (u *WorkerHTTPStreamSender) SendDelta(ctx context.Context, chatRunID, delta string) error {
	if chatRunID == "" {
		return nil
	}
	body := workerapi.StreamDeltaRequest{Delta: delta}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := u.BaseURL + "/api/worker/chat-runs/" + chatRunID + "/stream"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
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
		return fmt.Errorf("worker API POST stream %s: %s", url, resp.Status)
	}
	return nil
}
