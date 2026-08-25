package handlers

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/server/turnqueue"
	wsconn "github.com/gougoujiang/buildmax/internal/server/websocket"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
	convchannel "github.com/gougoujiang/buildmax/internal/service/conversation/channel"
)

const taskResultMaxOutputLen = 4000

// deliveryMaxAttempts bounds how many times one report is tried.
//
// A report that has failed this many times is failing for a reason retrying
// does not fix — a conversation nobody can load, a model the deployment no
// longer serves — and the outcome it would have summarized is on the run
// either way. Giving up is recorded, not silent.
const deliveryMaxAttempts = 5

// deliveryLease is how long a claimed report is left alone before another
// sweep may take it. It has to outlast a turn: a claim that expires while the
// model is still answering would let a second attempt report the same run.
const deliveryLease = 10 * time.Minute

// deliveryBackoff is how long to wait after a failed attempt. Flat rather than
// exponential: the failures worth retrying here are transient (a model call
// that timed out, a conversation briefly busy) and the ones that are not are
// bounded by deliveryMaxAttempts anyway.
const deliveryBackoff = time.Minute

// reportTaskRunTerminal is what a finished run does to the Portal.
//
// Two independent things, in this order. Everyone watching the team is told the
// task changed, so an open page reloads the run's card from the database rather
// than trusting whatever it last saw. Then the report owed to the conversation
// is recorded and attempted.
//
// The two halves do not depend on each other. A report that fails leaves the
// card standing, and a card nobody is connected to see does not stop the report
// from being made.
func (h *Handler) reportTaskRunTerminal(ctx context.Context, info coretask.RunTerminalInfo) {
	h.connRegistry.Broadcast(info.TeamID, info.UserID, wsconn.TypeTaskStatusChanged, wsconn.TaskStatusChanged{
		TaskID: info.TaskID,
		Status: info.Status,
	})
	if info.ConversationID == "" || h.cfg.ConversationLLMClient == nil {
		return
	}
	// Detached from the caller's context: the announcer's goroutine returns as
	// soon as the turn is queued, and a turn cancelled at that moment would lose
	// the report.
	reportCtx := context.WithoutCancel(ctx)
	if h.cfg.TaskResultDeliveries == nil {
		// No delivery store — a deployment without a database, or a test. The
		// report is still made; it just is not owed anywhere if it fails.
		h.submitTaskResultTurn(reportCtx, info, nil)
		return
	}
	if err := h.cfg.TaskResultDeliveries.EnqueueTaskResultDelivery(reportCtx, info.TaskRunID, info.ConversationID, time.Now().UTC()); err != nil {
		slog.Warn("task result delivery not recorded", "task_run_id", info.TaskRunID, "err", err)
	}
	h.attemptTaskResultDelivery(reportCtx, info.TaskRunID, time.Now())
}

// attemptTaskResultDelivery makes one attempt at one owed report.
//
// The claim is what keeps a run from being reported twice: the terminal
// callback and a sweep can both reach the same delivery, and only the claim
// that matched proceeds.
func (h *Handler) attemptTaskResultDelivery(ctx context.Context, taskRunID string, now time.Time) {
	claimed, err := h.cfg.TaskResultDeliveries.ClaimTaskResultDelivery(ctx, taskRunID, now, now.Add(deliveryLease))
	if err != nil {
		slog.Warn("task result delivery not claimed", "task_run_id", taskRunID, "err", err)
		return
	}
	if claimed == nil {
		return
	}
	info, ok := h.loadTerminalInfo(ctx, taskRunID)
	if !ok {
		// Nothing left to report from. Close it rather than retrying a read
		// that will keep failing the same way.
		h.finishDelivery(ctx, taskRunID, coretask.DeliveryAbandoned, "the run this report describes could not be read")
		return
	}
	if claimed.Attempts > deliveryMaxAttempts {
		h.finishDelivery(ctx, taskRunID, coretask.DeliveryAbandoned, "gave up after "+strconv.Itoa(deliveryMaxAttempts)+" attempts")
		return
	}
	h.submitTaskResultTurn(ctx, info, func(turnErr error) {
		if turnErr == nil {
			h.finishDelivery(ctx, taskRunID, coretask.DeliveryDelivered, "")
			return
		}
		// Left pending, and brought forward: the claim pushed the next attempt
		// out to cover a turn still running, and this one is no longer running.
		next := now.Add(deliveryBackoff)
		if err := h.cfg.TaskResultDeliveries.RecordTaskResultDeliveryFailure(ctx, taskRunID, turnErr.Error(), next); err != nil {
			slog.Warn("task result delivery failure not recorded", "task_run_id", taskRunID, "err", err)
		}
	})
}

func (h *Handler) finishDelivery(ctx context.Context, taskRunID, status, reason string) {
	var lastError *string
	if reason != "" {
		lastError = &reason
	}
	if err := h.cfg.TaskResultDeliveries.FinishTaskResultDelivery(ctx, taskRunID, status, lastError); err != nil {
		slog.Warn("task result delivery not closed", "task_run_id", taskRunID, "status", status, "err", err)
	}
	if status == coretask.DeliveryAbandoned {
		slog.Warn("gave up reporting a finished run", "task_run_id", taskRunID, "reason", reason)
	}
}

// loadTerminalInfo rebuilds what a report needs from the run itself.
//
// A sweep after a restart has nothing in memory, and re-reading is also what
// makes a retry report the run as it is rather than as it was when the first
// attempt was made.
func (h *Handler) loadTerminalInfo(ctx context.Context, taskRunID string) (coretask.RunTerminalInfo, bool) {
	if h.cfg.TaskRunStore == nil {
		return coretask.RunTerminalInfo{}, false
	}
	run, task, err := h.cfg.TaskRunStore.GetTaskRunWithTask(ctx, taskRunID)
	if err != nil || run == nil || task == nil {
		return coretask.RunTerminalInfo{}, false
	}
	return coretask.RunTerminalInfo{
		TaskRunID:      run.ID,
		TaskID:         run.TaskID,
		ConversationID: task.ConversationID,
		TeamID:         task.TeamID,
		UserID:         task.CreatedBy,
		Status:         run.Status,
		Output:         run.Output,
		ErrorMessage:   run.ErrorMessage,
	}, true
}

// submitTaskResultTurn queues the Tier 1 turn that reports one finished run.
//
// The turn goes to the server's turn queue, not to a socket. A run finishes
// whether or not the person who started it is still connected, and the reply
// waiting for them when they come back must not depend on that. onDone, when
// set, is called with the turn's outcome once it has run.
func (h *Handler) submitTaskResultTurn(ctx context.Context, info coretask.RunTerminalInfo, onDone func(error)) {
	message := formatTaskResultMessage(info)
	job := turnqueue.NewJob(func() {
		_, err := h.conversations.HandleTurn(ctx, conversation.HandleTurnCmd{
			UserID:         info.UserID,
			Channel:        convchannel.ChannelSystem,
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
		if onDone != nil {
			onDone(err)
		}
	})
	if _, err := h.turns.Submit(info.ConversationID, job); err != nil {
		slog.Warn("task result turn not queued", "conversation_id", info.ConversationID, "task_id", info.TaskID, "err", err)
		if onDone != nil {
			onDone(err)
		}
	}
}

func formatTaskResultMessage(info coretask.RunTerminalInfo) string {
	if info.Status == string(coretask.RunStatusCanceled) {
		// A cancel is an instruction, not a fault. Saying so keeps Tier 1 from
		// treating the stop as a failure worth retrying or apologising for.
		note := ""
		if info.ErrorMessage != nil && *info.ErrorMessage != "" {
			note = "\n\n" + *info.ErrorMessage
		}
		return "[Task Result] task_id: " + info.TaskID + " | status: canceled\n\nThis task was stopped on request." + note
	}
	if info.Status == string(coretask.RunStatusSucceeded) {
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
