package handlers

import (
	"context"
	"log/slog"
	"sync"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// connRegistry tracks active WebSocket connections per user.
type connRegistry struct {
	mu    sync.RWMutex
	conns map[string][]*wsConn
}

func newConnRegistry() *connRegistry {
	return &connRegistry{conns: make(map[string][]*wsConn)}
}

func (r *connRegistry) Register(userID string, c *wsConn) {
	r.mu.Lock()
	r.conns[userID] = append(r.conns[userID], c)
	r.mu.Unlock()
}

func (r *connRegistry) Unregister(userID string, c *wsConn) {
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

func (r *connRegistry) ForUser(userID string) []*wsConn {
	r.mu.RLock()
	list := r.conns[userID]
	out := make([]*wsConn, len(list))
	copy(out, list)
	r.mu.RUnlock()
	return out
}

const taskResultMaxOutputLen = 4000

// OnTaskRunTerminal is called when a task run reaches terminal status. It finds the user's active
// WebSocket connection and triggers a system Tier 1 conversation turn with the task result.
func (r *connRegistry) OnTaskRunTerminal(ctx context.Context, info TaskRunTerminalInfo) {
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

func formatTaskResultMessage(info TaskRunTerminalInfo) string {
	if info.Status == string(model.RunStatusSucceeded) {
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
