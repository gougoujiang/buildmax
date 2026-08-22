package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/service/task"
	"github.com/gougoujiang/buildmax/internal/util"
)

func NewStartTaskServiceRunner(taskService *task.Service, conversationID, teamID, userID string) StartTaskRunner {
	if taskService == nil {
		return nil
	}
	return &startTaskServiceRunner{
		taskService:    taskService,
		userID:         userID,
		conversationID: conversationID,
		teamID:         teamID,
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

func NewContinueTaskServiceRunner(taskService *task.Service) ContinueTaskRunner {
	if taskService == nil {
		return nil
	}
	return &continueTaskServiceRunner{taskService: taskService}
}

type startTaskServiceRunner struct {
	taskService    *task.Service
	userID         string
	conversationID string
	teamID         string
}

func (r *startTaskServiceRunner) StartTask(ctx context.Context, input string, agentID *string) (taskID, runID string, err error) {
	if r.teamID == "" {
		return "", "", fmt.Errorf("conversation has no team")
	}
	result, err := r.taskService.StartBackgroundTask(ctx, task.CreateTaskCmd{
		ConversationID: r.conversationID,
		UserID:         r.userID,
		TeamID:         r.teamID,
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
		snippet := util.TruncateRunes(item.Input, 60)
		if item.Title != "" {
			snippet = util.TruncateRunes(item.Title, 60)
		}
		ts := util.FormatUnixMinute(item.CreatedAt)
		lines = append(lines, fmt.Sprintf("%d. %s | %s | %s | %s", i+1, item.ID, snippet, item.Status, ts))
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
	inputTrunc := util.TruncateRunes(taskItem.Input, 500)
	outputLine := ""
	if taskItem.Output != nil && *taskItem.Output != "" {
		outputLine = "output_snippet: " + util.TruncateRunes(*taskItem.Output, 200) + "\n"
	}
	lastRun := ""
	if taskItem.LastRunID != nil {
		lastRun = *taskItem.LastRunID
	}
	return fmt.Sprintf("task_id: %s\ntitle: %s\ninput: %s\nstatus: %s\ncreated_at: %s\nlast_run_id: %s\n%s",
		taskItem.ID, taskItem.Title, inputTrunc, taskItem.Status, util.FormatUnixMinute(taskItem.CreatedAt), lastRun, outputLine), nil
}

type continueTaskServiceRunner struct {
	taskService *task.Service
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
	return run.ID, nil
}
