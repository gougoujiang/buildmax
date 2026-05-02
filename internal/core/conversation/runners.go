package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"buildmax/internal/core/model"
	taskapp "buildmax/internal/core/task"
)

// startTaskRunner implements StartTaskRunner by creating a background task via taskService.
type startTaskRunner struct {
	taskService       *taskapp.Service
	conversationStore model.ConversationStore
	userID            string
	conversationID    string
}

func (r *startTaskRunner) StartTask(ctx context.Context, _, _, input string, agentID *string) (taskID, runID string, err error) {
	conv, err := r.conversationStore.GetConversation(ctx, r.conversationID)
	if err != nil {
		return "", "", err
	}
	if conv == nil {
		return "", "", fmt.Errorf("conversation not found")
	}
	result, err := r.taskService.StartBackgroundTask(ctx, taskapp.CreateTaskCmd{
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

// listTasksRunner implements ListTasksRunner by querying the task store.
type listTasksRunner struct {
	tasks model.TaskStore
}

func (r *listTasksRunner) ListTasks(ctx context.Context, conversationID string) (string, error) {
	list, _, err := r.tasks.ListTasksByConversationPaginated(ctx, conversationID, false, 10, 0)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "No recent tasks in this conversation.", nil
	}
	var lines []string
	for i, c := range list {
		snippet := taskTitleOrSnippet(&c, 60)
		ts := formatCreatedAt(c.CreatedAt)
		lines = append(lines, fmt.Sprintf("%d. %s | %s | %s | %s", i+1, c.TaskID, snippet, c.Status, ts))
	}
	return strings.Join(lines, "\n"), nil
}

// getTaskRunner implements GetTaskRunner by querying the task store.
type getTaskRunner struct {
	tasks model.TaskStore
}

func (r *getTaskRunner) GetTask(ctx context.Context, conversationID, taskID string) (string, error) {
	task, err := r.tasks.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	if task == nil {
		return "", fmt.Errorf("task not found or not in this conversation")
	}
	if task.ConversationID != conversationID {
		return "", fmt.Errorf("task not found or not in this conversation")
	}
	inputTrunc := truncateRunes(task.Input, 500)
	outputLine := ""
	if task.Output != nil && *task.Output != "" {
		outputLine = "output_snippet: " + truncateRunes(*task.Output, 200) + "\n"
	}
	lastRun := ""
	if task.LastRunID != nil {
		lastRun = *task.LastRunID
	}
	return fmt.Sprintf("task_id: %s\ntitle: %s\ninput: %s\nstatus: %s\ncreated_at: %s\nlast_run_id: %s\n%s",
		task.TaskID, task.Title, inputTrunc, task.Status, formatCreatedAt(task.CreatedAt), lastRun, outputLine), nil
}

// continueTaskRunner implements ContinueTaskRunner by creating a new run on an existing task.
type continueTaskRunner struct {
	taskService *taskapp.Service
}

func (r *continueTaskRunner) ContinueTask(ctx context.Context, conversationID, userID, taskID, input string) (runID string, err error) {
	if r.taskService.Tasks == nil {
		return "", fmt.Errorf("tasks not configured")
	}
	task, err := r.taskService.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	if task == nil || task.ConversationID != conversationID {
		return "", fmt.Errorf("task not found or not in this conversation")
	}
	run, err := r.taskService.CreateRun(ctx, taskapp.CreateRunCmd{
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

func taskTitleOrSnippet(task *model.Task, maxRunes int) string {
	if task.Title != "" {
		return truncateRunes(task.Title, maxRunes)
	}
	return truncateRunes(task.Input, maxRunes)
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
