package websocket

import (
	"context"
	"sync"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// ConnRegistry tracks active WebSocket connections per user.
type ConnRegistry struct {
	mu    sync.RWMutex
	conns map[string][]*Conn
}

func NewConnRegistry() *ConnRegistry {
	return &ConnRegistry{conns: make(map[string][]*Conn)}
}

func (r *ConnRegistry) Register(userID string, c *Conn) {
	r.mu.Lock()
	r.conns[userID] = append(r.conns[userID], c)
	r.mu.Unlock()
}

func (r *ConnRegistry) Unregister(userID string, c *Conn) {
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

func (r *ConnRegistry) ForUser(userID string) []*Conn {
	r.mu.RLock()
	list := r.conns[userID]
	out := make([]*Conn, len(list))
	copy(out, list)
	r.mu.RUnlock()
	return out
}

const taskResultMaxOutputLen = 4000

// OnTaskRunTerminal is called when a task run reaches terminal status. It finds the user's active
// WebSocket connection and triggers a system Tier 1 conversation turn with the task result.
func (r *ConnRegistry) OnTaskRunTerminal(ctx context.Context, info model.TaskRunTerminalInfo) {
	conns := r.ForUser(info.UserID)
	if len(conns) == 0 {
		componentLog().Info("user not connected, skipping", "user_id", info.UserID, "task_id", info.TaskID)
		return
	}
	msg := formatTaskResultMessage(info)
	wc := conns[0]
	componentLog().Info("triggering system turn", "user_id", info.UserID, "conversation_id", info.ConversationID, "task_id", info.TaskID, "status", info.Status)
	wc.RunSystemConversationTurn(ctx, info.ConversationID, msg)
}

func formatTaskResultMessage(info model.TaskRunTerminalInfo) string {
	if info.Status == string(model.RunStatusCanceled) {
		// A cancel is an instruction, not a fault. Saying so keeps Tier 1 from
		// treating the stop as a failure worth retrying or apologising for.
		note := ""
		if info.ErrorMessage != nil && *info.ErrorMessage != "" {
			note = "\n\n" + *info.ErrorMessage
		}
		return "[Task Result] task_id: " + info.TaskID + " | status: canceled\n\nThis task was stopped on request." + note
	}
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
