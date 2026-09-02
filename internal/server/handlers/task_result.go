package handlers

import (
	"context"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	wsconn "github.com/gougoujiang/buildmax/internal/server/websocket"
)

// reportTaskRunTerminal announces durable state. TaskRun output is already the
// authoritative result; completion never creates a Conversation turn.
func (h *Handler) reportTaskRunTerminal(_ context.Context, info coretask.RunTerminalInfo) {
	h.connRegistry.Broadcast(info.TeamID, info.UserID, wsconn.TypeTaskStatusChanged, wsconn.TaskStatusChanged{
		TaskID: info.TaskID,
		Status: info.Status,
	})
}
