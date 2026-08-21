// Package runterminal announces a task run that reached a terminal status.
package runterminal

import (
	"context"
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/websocket"
)

// Announcer closes out a run: it tells the stream hub the task is done and
// hands the outcome to whoever the server wired up.
type Announcer struct {
	Runs model.TaskRunStore
	Hub  websocket.StreamHub
	// On receives the outcome. Nil means nobody is listening beyond the hub.
	On func(ctx context.Context, info model.TaskRunTerminalInfo)
}

// Every terminal outcome goes through here — reported by a worker or written by
// the server on a cancel — because a conversation that started a task waits for
// exactly one of these. Losing it leaves the conversation waiting for a run that
// has already stopped.
// Announce tells everyone watching that a run finished.
//
// Two things trigger it: a worker reporting its own outcome, and a user
// cancelling from the Portal. Both must reach the same listeners in the same
// order, so it is one mechanism rather than a copy on each path.
func (a *Announcer) Announce(ctx context.Context, taskRunID, status string, output, errorMessage *string) {
	run, task, _ := a.Runs.GetTaskRunWithTask(ctx, taskRunID)
	if run == nil {
		return
	}
	a.Hub.Done(run.TaskID)
	if task == nil {
		return
	}
	info := model.TaskRunTerminalInfo{
		TaskRunID:      run.TaskRunID,
		TaskID:         run.TaskID,
		ConversationID: task.ConversationID,
		UserID:         task.CreatedBy,
		Status:         status,
		Output:         output,
		ErrorMessage:   errorMessage,
	}
	go func() {
		slog.Default().Info("firing task run terminal callbacks", "task_run_id", info.TaskRunID, "status", info.Status)
		if a.On != nil {
			a.On(context.Background(), info)
		}
	}()
}
