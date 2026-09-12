package conversation

import (
	"context"
	"fmt"
	"strings"

	coretask "github.com/icloudbb/buildmax/internal/core/task"
	"github.com/icloudbb/buildmax/internal/service/task"
	"github.com/icloudbb/buildmax/internal/util"
)

// sourceMessageID is bound per turn because the model must not choose, or omit,
// the message that requested the work.
func newStartTaskServiceRunner(taskService *task.Service, conversationID, spaceID, userID string, sourceMessageID *string) startTaskRunner {
	if taskService == nil {
		return nil
	}
	return &startTaskServiceRunner{
		taskService:     taskService,
		userID:          userID,
		conversationID:  conversationID,
		spaceID:         spaceID,
		sourceMessageID: sourceMessageID,
	}
}

func newListTasksStoreRunner(tasks coretask.Store) listTasksRunner {
	if tasks == nil {
		return nil
	}
	return &listTasksStoreRunner{tasks: tasks}
}

func newGetTaskServiceRunner(taskService *task.Service) getTaskRunner {
	if taskService == nil || taskService.Tasks == nil {
		return nil
	}
	return &getTaskServiceRunner{taskService: taskService}
}

// sourceMessageID is bound per turn for the same reason as StartTask: each
// continuation run records the message that requested it.
func newContinueTaskServiceRunner(taskService *task.Service, sourceMessageID *string) continueTaskRunner {
	if taskService == nil {
		return nil
	}
	return &continueTaskServiceRunner{taskService: taskService, sourceMessageID: sourceMessageID}
}

type startTaskServiceRunner struct {
	taskService     *task.Service
	userID          string
	conversationID  string
	spaceID         string
	sourceMessageID *string
}

func (r *startTaskServiceRunner) StartTask(ctx context.Context, input string, agentID *string) (taskID, runID string, err error) {
	if r.spaceID == "" {
		return "", "", fmt.Errorf("conversation has no space")
	}
	result, err := r.taskService.StartBackgroundTask(ctx, task.CreateTaskCmd{
		ConversationID:  r.conversationID,
		UserID:          r.userID,
		SpaceID:         r.spaceID,
		Input:           input,
		AgentID:         agentID,
		CreatedByType:   coretask.RunCreatedByTypeUser,
		TriggerSource:   coretask.RunTriggerSourcePortalConversation,
		SourceMessageID: r.sourceMessageID,
	})
	if err != nil {
		return "", "", err
	}
	return result.TaskID, result.RunID, nil
}

type listTasksStoreRunner struct {
	tasks coretask.Store
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
		ts := util.FormatMinute(item.CreatedAt)
		lines = append(lines, fmt.Sprintf("%d. %s | %s | %s | %s", i+1, item.ID, snippet, item.Status, ts))
	}
	return strings.Join(lines, "\n"), nil
}

type getTaskServiceRunner struct {
	taskService *task.Service
}

func (r *getTaskServiceRunner) GetTask(ctx context.Context, conversationID, taskID string) (string, error) {
	taskItem, err := r.taskService.GetTaskInConversation(ctx, conversationID, taskID)
	if err != nil {
		return "", err
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
		taskItem.ID, taskItem.Title, inputTrunc, taskItem.Status, util.FormatMinute(taskItem.CreatedAt), lastRun, outputLine), nil
}

type continueTaskServiceRunner struct {
	taskService     *task.Service
	sourceMessageID *string
}

func (r *continueTaskServiceRunner) ContinueTask(ctx context.Context, conversationID, userID, taskID, input string) (runID string, err error) {
	if _, err := r.taskService.GetTaskInConversation(ctx, conversationID, taskID); err != nil {
		return "", err
	}
	run, err := r.taskService.CreateRun(ctx, task.CreateRunCmd{
		UserID:          userID,
		TaskID:          taskID,
		Input:           input,
		CreatedByType:   coretask.RunCreatedByTypeUser,
		TriggerSource:   coretask.RunTriggerSourcePortalConversation,
		SourceMessageID: r.sourceMessageID,
	})
	if err != nil {
		return "", err
	}
	return run.ID, nil
}
