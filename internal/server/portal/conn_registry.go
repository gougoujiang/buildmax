package portal

import (
	"context"
	"log/slog"
	"sync"

	"buildmax/internal/server/worker"
	"buildmax/internal/storage/entity"
)

// ConnRegistry tracks active WebSocket connections per user.
// Thread-safe; used by the WS handler (register/unregister) and
// by the task-completion callback (lookup by user ID).
type ConnRegistry struct {
	mu    sync.RWMutex
	conns map[string][]*wsConn
}

// NewConnRegistry returns an empty registry.
func NewConnRegistry() *ConnRegistry {
	return &ConnRegistry{conns: make(map[string][]*wsConn)}
}

// Register adds a connection for the given user.
func (r *ConnRegistry) Register(userID string, c *wsConn) {
	r.mu.Lock()
	r.conns[userID] = append(r.conns[userID], c)
	r.mu.Unlock()
}

// Unregister removes a specific connection for the given user.
func (r *ConnRegistry) Unregister(userID string, c *wsConn) {
	r.mu.Lock()
	list := r.conns[userID]
	for i, cc := range list {
		if cc == c {
			r.conns[userID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(r.conns[userID]) == 0 {
		delete(r.conns, userID)
	}
	r.mu.Unlock()
}

// ForUser returns a snapshot of active connections for the given user.
// Returns nil if the user has no active connections.
func (r *ConnRegistry) ForUser(userID string) []*wsConn {
	r.mu.RLock()
	list := r.conns[userID]
	out := make([]*wsConn, len(list))
	copy(out, list)
	r.mu.RUnlock()
	return out
}

const taskResultMaxOutputLen = 4000

// OnTaskRunTerminal is the callback fired when a task run reaches terminal status.
// It looks up the user's active WebSocket connection and runs a system-triggered
// Tier 1 conversation turn so the user receives the task result in the conversation.
func (r *ConnRegistry) OnTaskRunTerminal(ctx context.Context, info worker.TaskRunTerminalInfo) {
	conns := r.ForUser(info.UserID)
	if len(conns) == 0 {
		slog.Info("task run terminal: user not connected, skipping", "user_id", info.UserID, "task_id", info.TaskID)
		return
	}
	msg := formatTaskResultMessage(info)
	wc := conns[0]
	slog.Info("task run terminal: triggering system turn", "user_id", info.UserID, "conversation_id", info.ConversationID, "task_id", info.TaskID, "status", info.Status)
	wc.RunSystemConversationTurn(ctx, info.ConversationID, msg)
}

func formatTaskResultMessage(info worker.TaskRunTerminalInfo) string {
	if info.Status == string(entity.RunStatusSucceeded) {
		output := ""
		if info.Output != nil {
			output = *info.Output
		}
		if len(output) > taskResultMaxOutputLen {
			output = output[:taskResultMaxOutputLen] + "\n...(truncated)"
		}
		return "[Task Result] task_id: " + info.TaskID + " | status: succeeded\n\n" + output
	}
	errMsg := "unknown error"
	if info.ErrorMessage != nil && *info.ErrorMessage != "" {
		errMsg = *info.ErrorMessage
	}
	return "[Task Result] task_id: " + info.TaskID + " | status: failed\n\nError: " + errMsg
}
