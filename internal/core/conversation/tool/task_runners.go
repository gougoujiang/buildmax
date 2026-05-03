package tool

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"buildmax/internal/core/model"
	"buildmax/internal/core/task"
)

func NewStartTaskServiceRunner(taskService *task.TaskService, conversationStore model.ConversationStore, conversationID, userID string) StartTaskRunner {
	if taskService == nil || conversationStore == nil {
		return nil
	}
	return &startTaskServiceRunner{
		taskService:       taskService,
		conversationStore: conversationStore,
		userID:            userID,
		conversationID:    conversationID,
	}
}

func NewListTasksStoreRunner(tasks model.TaskStore) ListTasksRunner {
	if tasks == nil {
		return nil
	}
	return &listTasksStoreRunner{tasks: tasks}
}

func NewGetTaskStoreRunner(tasks model.TaskStore) GetTaskRunner {
	if tasks == nil {
		return nil
	}
	return &getTaskStoreRunner{tasks: tasks}
}

func NewContinueTaskServiceRunner(taskService *task.TaskService) ContinueTaskRunner {
	if taskService == nil {
		return nil
	}
	return &continueTaskServiceRunner{taskService: taskService}
}

type startTaskServiceRunner struct {
	taskService       *task.TaskService
	conversationStore model.ConversationStore
	userID            string
	conversationID    string
}

func (r *startTaskServiceRunner) StartTask(ctx context.Context, _, _, input string, agentID *string) (taskID, runID string, err error) {
	conv, err := r.conversationStore.GetConversation(ctx, r.conversationID)
	if err != nil {
		return "", "", err
	}
	if conv == nil {
		return "", "", fmt.Errorf("conversation not found")
	}
	result, err := r.taskService.StartBackgroundTask(ctx, task.CreateTaskCmd{
		ConversationID: r.conversationID,
		UserID:         r.userID,
		TeamID:         conv.TeamID,
		Input:          input,
		AgentID:        agentID,
		CreatedByType:  model.RunCreatedByTypeUser,
		TriggerSource:  model.RunTriggerSourcePortalConversation,
	})
	if err != nil {
		return "", "", err
	}
	return result.TaskID, result.RunID, nil
}

type listTasksStoreRunner struct {
	tasks model.TaskStore
}

func (r *listTasksStoreRunner) ListTasks(ctx context.Context, conversationID string) (string, error) {
	list, _, err := r.tasks.ListTasksByConversationPaginated(ctx, conversationID, false, 10, 0)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "No recent tasks in this conversation.", nil
	}
	var lines []string
	for i, item := range list {
		snippet := taskTitleOrSnippet(&item, 60)
		ts := formatCreatedAt(item.CreatedAt)
		lines = append(lines, fmt.Sprintf("%d. %s | %s | %s | %s", i+1, item.TaskID, snippet, item.Status, ts))
	}
	return strings.Join(lines, "\n"), nil
}

type getTaskStoreRunner struct {
	tasks model.TaskStore
}

func (r *getTaskStoreRunner) GetTask(ctx context.Context, conversationID, taskID string) (string, error) {
	taskItem, err := r.tasks.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	if taskItem == nil || taskItem.ConversationID != conversationID {
		return "", fmt.Errorf("task not found or not in this conversation")
	}
	inputTrunc := truncateRunes(taskItem.Input, 500)
	outputLine := ""
	if taskItem.Output != nil && *taskItem.Output != "" {
		outputLine = "output_snippet: " + truncateRunes(*taskItem.Output, 200) + "\n"
	}
	lastRun := ""
	if taskItem.LastRunID != nil {
		lastRun = *taskItem.LastRunID
	}
	return fmt.Sprintf("task_id: %s\ntitle: %s\ninput: %s\nstatus: %s\ncreated_at: %s\nlast_run_id: %s\n%s",
		taskItem.TaskID, taskItem.Title, inputTrunc, taskItem.Status, formatCreatedAt(taskItem.CreatedAt), lastRun, outputLine), nil
}

type continueTaskServiceRunner struct {
	taskService *task.TaskService
}

func (r *continueTaskServiceRunner) ContinueTask(ctx context.Context, conversationID, userID, taskID, input string) (runID string, err error) {
	if r.taskService.Tasks == nil {
		return "", fmt.Errorf("tasks not configured")
	}
	taskItem, err := r.taskService.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	if taskItem == nil || taskItem.ConversationID != conversationID {
		return "", fmt.Errorf("task not found or not in this conversation")
	}
	run, err := r.taskService.CreateRun(ctx, task.CreateRunCmd{
		UserID:        userID,
		TaskID:        taskID,
		Input:         input,
		CreatedByType: model.RunCreatedByTypeUser,
		TriggerSource: model.RunTriggerSourcePortalConversation,
	})
	if err != nil {
		return "", err
	}
	return run.TaskRunID, nil
}

func taskTitleOrSnippet(taskItem *model.Task, maxRunes int) string {
	if taskItem.Title != "" {
		return truncateRunes(taskItem.Title, maxRunes)
	}
	return truncateRunes(taskItem.Input, maxRunes)
}

func truncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}

func formatCreatedAt(unixSec int64) string {
	return time.Unix(unixSec, 0).Format("2006-01-02 15:04")
}
