package workerapi

import (
	"context"

	"buildmax/internal/core/model"
	streamhub "buildmax/internal/server/websocket"
)

// TaskRunTerminalInfo holds information about a task run that reached terminal status.
type TaskRunTerminalInfo struct {
	TaskRunID      string
	TaskID         string
	ConversationID string
	UserID         string
	Status         string  // "succeeded" or "failed"
	Output         *string // task output (succeeded) — may be nil
	ErrorMessage   *string // error message (failed) — may be nil
}

// Config holds dependencies for the worker API (task-run get, patch, stream).
type Config struct {
	Token             string                                              // Required for Authorization: Bearer <token> or X-Worker-Token
	TaskRunStore      model.TaskRunStore                                  // Required for get/patch/stream
	Hub               streamhub.StreamHub                                 // Optional; required for POST .../stream and for Done on patch
	OnTaskRunTerminal func(ctx context.Context, info TaskRunTerminalInfo) // Optional; fired when a run reaches terminal status
}
