package handlers

import (
	"context"
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/turnqueue"
	wsconn "github.com/gougoujiang/buildmax/internal/server/websocket"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
)

const taskResultMaxOutputLen = 4000

// reportTaskRunTerminal is what a finished run does to the Portal.
//
// Two independent things, in this order. Everyone watching the team is told the
// task changed, so an open page reloads the run's card from the database rather
// than trusting whatever it last saw. Then one Tier 1 turn reports the outcome
// in the conversation that asked for it.
//
// The turn is submitted to the server's turn queue, not to a socket. A run
// finishes whether or not the person who started it is still connected, and the
// reply waiting for them when they come back must not depend on that. Neither
// half depends on the other: a turn that fails leaves the card standing, and a
// card nobody is connected to see does not stop the turn from running.
func (h *Handler) reportTaskRunTerminal(ctx context.Context, info model.TaskRunTerminalInfo) {
	h.connRegistry.Broadcast(info.TeamID, info.UserID, wsconn.TypeTaskStatusChanged, wsconn.TaskStatusChanged{
		TaskID: info.TaskID,
		Status: info.Status,
	})
	if info.ConversationID == "" || h.cfg.ConversationLLMClient == nil {
		return
	}
	h.submitTaskResultTurn(ctx, info)
}

func (h *Handler) submitTaskResultTurn(ctx context.Context, info model.TaskRunTerminalInfo) {
	// Detached from the caller's context: the announcer's goroutine returns as
	// soon as the job is queued, and a turn cancelled at that moment would lose
	// the report entirely.
	turnCtx := context.WithoutCancel(ctx)
	message := formatTaskResultMessage(info)
	job := turnqueue.NewJob(func() {
		_, err := h.conversationService().HandleTurn(turnCtx, conversation.HandleTurnCmd{
			UserID:         info.UserID,
			Channel:        conversation.ChannelSystem,
			Message:        message,
			ConversationID: info.ConversationID,
			StreamSink:     h.connRegistry.BroadcastSink(info.TeamID, info.UserID, info.ConversationID),
		})
		if err != nil {
			slog.Error("task result turn failed", "conversation_id", info.ConversationID, "task_id", info.TaskID, "err", err)
		}
		h.connRegistry.Broadcast(info.TeamID, info.UserID, wsconn.TypeMessageCompleted, wsconn.MessageCompleted{
			ConversationID:  info.ConversationID,
			QueuedRemaining: h.turns.Waiting(info.ConversationID),
		})
	})
	if _, err := h.turns.Submit(info.ConversationID, job); err != nil {
		slog.Warn("task result turn dropped: queue full", "conversation_id", info.ConversationID, "task_id", info.TaskID, "err", err)
	}
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
