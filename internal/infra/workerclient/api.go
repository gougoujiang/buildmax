package workerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/httpclient"
)

// StreamSender sends run output deltas to the server (e.g. for live streaming). Optional; when nil, run output is not streamed.
// Flush sends any buffered data; call when the stream ends so the last chunk is not lost.
type StreamSender interface {
	SendDelta(ctx context.Context, taskRunID, delta string) error
	Flush(ctx context.Context, taskRunID string) error
}

// ErrTaskRunAlreadyClaimed is returned when the server responds 409 to PATCH RUNNING (run not SCHEDULED or already RUNNING).
var ErrTaskRunAlreadyClaimed = errors.New("task run already claimed or not scheduled")

// WorkerAPIClientConfig holds base URL, token, and HTTP client for worker API calls.
type WorkerAPIClientConfig struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// workerDo performs an HTTP request to the worker API (URL build, Bearer auth, default client). Caller must close resp.Body.
func workerDo(ctx context.Context, cfg WorkerAPIClientConfig, method, pathSuffix string, body []byte) (*http.Response, error) {
	url := cfg.BaseURL + pathSuffix
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	if body != nil && (method == http.MethodPatch || method == http.MethodPost) {
		req.Header.Set("Content-Type", "application/json")
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// WorkerTaskRun is everything the server tells a worker about the run it is
// about to execute.
type WorkerTaskRun struct {
	Run  *model.TaskRun
	Task *model.Task
	// LLM is how this run reaches a model. Nil means direct, which is what a
	// server that has not enabled managed worker inference reports.
	LLM *TaskRunLLM
	// AgentInstructions is the text appended to the run's system prompt, resolved by the
	// server from the agent the task names. Empty when the task names none. It is not on
	// model.Task because it is not a property of the task: it is resolved per run, so an
	// edited definition applies to the next one.
	AgentInstructions string
	// CancelRequested is true when the run was already asked to stop before
	// this worker picked it up — a cancel that landed between dispatch and
	// start. Such a run is finished without executing anything.
	CancelRequested bool
}

// GetWorkerTaskRun fetches the run from the server (GET /api/worker/task-runs/{task_run_id}). Returns nil, nil if not found.
func GetWorkerTaskRun(ctx context.Context, cfg WorkerAPIClientConfig, taskRunID string) (*WorkerTaskRun, error) {
	pathSuffix := "/api/worker/task-runs/" + taskRunID
	resp, err := workerDo(ctx, cfg, http.MethodGet, pathSuffix, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, httpclient.DecodeError(resp, "worker API GET "+cfg.BaseURL+pathSuffix)
	}
	var got GetTaskRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		return nil, err
	}
	return &WorkerTaskRun{
		Run: &model.TaskRun{
			ID:        got.Run.ID,
			TaskID:    got.Run.TaskID,
			Input:     got.Run.Input,
			Status:    got.Run.Status,
			CreatedAt: got.Run.CreatedAt,
		},
		Task: &model.Task{
			ID:             got.Task.ID,
			ConversationID: got.Task.ConversationID,
			TeamID:         got.Task.TeamID,
			CreatedBy:      got.Task.UserID,
			SessionID:      got.Task.SessionID,
			LastRunID:      got.Task.LastRunID,
		},
		LLM:               got.LLM,
		AgentInstructions: got.Task.AgentInstructions,
		CancelRequested:   got.Run.CancelRequested,
	}, nil
}

// WorkerHTTPUpdater implements TaskRunUpdater by calling the server's worker API (PATCH /api/worker/task-runs/{task_run_id}).
type WorkerHTTPUpdater struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// UpdateRunStatus sends PATCH to the server to update run status and optional fields.
func (u *WorkerHTTPUpdater) UpdateRunStatus(ctx context.Context, taskRunID string, req *PatchTaskRunRequest) error {
	if req == nil {
		req = &PatchTaskRunRequest{}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	cfg := WorkerAPIClientConfig{BaseURL: u.BaseURL, Token: u.Token, Client: u.Client}
	pathSuffix := "/api/worker/task-runs/" + taskRunID
	resp, err := workerDo(ctx, cfg, http.MethodPatch, pathSuffix, raw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return ErrTaskRunAlreadyClaimed
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpclient.DecodeError(resp, "worker API PATCH "+cfg.BaseURL+pathSuffix)
	}
	return nil
}

// WorkerHTTPStreamSender implements StreamSender by POSTing each delta to the server's worker stream endpoint.
type WorkerHTTPStreamSender struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// SendDelta POSTs the delta to POST /api/worker/task-runs/{task_run_id}/stream.
func (u *WorkerHTTPStreamSender) SendDelta(ctx context.Context, taskRunID, delta string) error {
	if taskRunID == "" {
		return nil
	}
	body := StreamDeltaRequest{Delta: delta}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	cfg := WorkerAPIClientConfig{BaseURL: u.BaseURL, Token: u.Token, Client: u.Client}
	pathSuffix := "/api/worker/task-runs/" + taskRunID + "/stream"
	resp, err := workerDo(ctx, cfg, http.MethodPost, pathSuffix, raw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpclient.DecodeError(resp, "worker API POST stream "+cfg.BaseURL+pathSuffix)
	}
	return nil
}

// Flush is a no-op; WorkerHTTPStreamSender sends each delta immediately.
func (u *WorkerHTTPStreamSender) Flush(ctx context.Context, taskRunID string) error {
	return nil
}

// DebouncedStreamSender wraps a StreamSender and buffers deltas, flushing after an interval or when buffer size is reached.
// Reduces server API pressure and gives a smoother typewriter-like experience on the client.
const (
	DebounceIntervalMs = 80  // flush at most every 80ms when there is buffered data
	DebounceMaxBytes   = 512 // flush when buffer reaches 512 bytes
)

// DebouncedStreamSender implements StreamSender with debouncing.
type DebouncedStreamSender struct {
	Inner      StreamSender
	IntervalMs int // override default when > 0
	MaxBytes   int // override default when > 0

	mu               sync.Mutex
	buf              strings.Builder
	currentTaskRunID string
	timer            *time.Timer
}

func (d *DebouncedStreamSender) SendDelta(ctx context.Context, taskRunID, delta string) error {
	return d.sendOrFlush(ctx, taskRunID, delta, false)
}

func (d *DebouncedStreamSender) Flush(ctx context.Context, taskRunID string) error {
	return d.sendOrFlush(ctx, taskRunID, "", true)
}

func (d *DebouncedStreamSender) sendOrFlush(ctx context.Context, taskRunID, delta string, forceFlush bool) error {
	intervalMs := d.IntervalMs
	if intervalMs <= 0 {
		intervalMs = DebounceIntervalMs
	}
	maxBytes := d.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DebounceMaxBytes
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if forceFlush {
		d.flushLocked(ctx)
		return nil
	}

	if delta == "" {
		return nil
	}
	if d.buf.Len() == 0 {
		d.currentTaskRunID = taskRunID
	}
	d.buf.WriteString(delta)

	if d.buf.Len() >= maxBytes {
		d.flushLocked(ctx)
		return nil
	}

	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	d.timer = time.AfterFunc(time.Duration(intervalMs)*time.Millisecond, func() {
		d.mu.Lock()
		d.flushLocked(context.Background())
		d.mu.Unlock()
	})
	return nil
}

// flushLocked sends the current buffer via Inner and clears it. Caller must hold d.mu.
func (d *DebouncedStreamSender) flushLocked(ctx context.Context) {
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	if d.buf.Len() == 0 {
		return
	}
	payload := d.buf.String()
	runID := d.currentTaskRunID
	d.buf.Reset()
	d.mu.Unlock()
	defer d.mu.Lock()
	_ = d.Inner.SendDelta(ctx, runID, payload)
}
