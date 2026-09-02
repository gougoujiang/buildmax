package db

import (
	"testing"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
)

// A direct Agent run names a Team and an Agent and no Conversation at all —
// the shape agent-execution-and-task-threads.md §4.1 and §8.1 describe.
// team_id is required and authoritative; conversation_id is absent, not just
// empty on the wire.
func TestCreateTaskDirectHasNoConversation(t *testing.T) {
	s, ctx := newTestStore(t)
	userID := newTestUser(t, s, "direct-task")
	teamID := newTestTeam(t, s, userID)

	agent, err := s.CreateAgentInTeam(ctx, agentdef.CreateInput{
		TeamID: teamID, UserID: userID, Def: agentdef.Definition{Name: "direct-runner"},
	})
	if err != nil {
		t.Fatalf("CreateAgentInTeam: %v", err)
	}

	task, err := s.CreateTask(ctx, &coretask.CreateInput{
		TeamID:    teamID,
		AgentID:   &agent.ID,
		Input:     "run the nightly checks",
		CreatedBy: userID,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&taskRunRow{}, "task_id = ?", task.ID)
		_ = s.db.WithContext(ctx).Delete(&taskRow{}, "task_id = ?", task.ID)
	})

	if task.TeamID != teamID {
		t.Errorf("task.TeamID = %q, want %q", task.TeamID, teamID)
	}
	if task.ConversationID != "" {
		t.Errorf("task.ConversationID = %q, want empty for a direct run", task.ConversationID)
	}
	if task.AgentID == nil || *task.AgentID != agent.ID {
		t.Errorf("task.AgentID = %v, want %s", task.AgentID, agent.ID)
	}
	if task.LastRunID == nil {
		t.Fatal("CreateTask should create the first run")
	}

	got, err := s.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got == nil {
		t.Fatal("GetTask returned nil for a task that was just created")
	}
	if got.ConversationID != "" {
		t.Errorf("re-read task.ConversationID = %q, want empty", got.ConversationID)
	}
	if got.TeamID != teamID {
		t.Errorf("re-read task.TeamID = %q, want %q", got.TeamID, teamID)
	}

	list, total, err := s.ListTasksByAgent(ctx, teamID, agent.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListTasksByAgent: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].ID != task.ID {
		t.Fatalf("ListTasksByAgent = %+v (total %d), want just %s", list, total, task.ID)
	}

	// A second team's agent cannot see it: team ownership is authoritative,
	// not the agent id alone.
	strangerUser := newTestUser(t, s, "direct-task-stranger")
	strangerTeam := newTestTeam(t, s, strangerUser)
	if strangerList, strangerTotal, err := s.ListTasksByAgent(ctx, strangerTeam, agent.ID, 10, 0); err != nil {
		t.Fatalf("ListTasksByAgent (stranger team): %v", err)
	} else if strangerTotal != 0 || len(strangerList) != 0 {
		t.Errorf("a stranger team's agent lookup saw %d tasks, want 0", strangerTotal)
	}
}

// Continue on a direct-run Task creates a second TaskRun on the same Task and
// still needs no Conversation.
func TestCreateTaskRunContinuesADirectTaskWithoutAConversation(t *testing.T) {
	s, ctx := newTestStore(t)
	userID := newTestUser(t, s, "direct-continue")
	teamID := newTestTeam(t, s, userID)

	agent, err := s.CreateAgentInTeam(ctx, agentdef.CreateInput{
		TeamID: teamID, UserID: userID, Def: agentdef.Definition{Name: "continuable"},
	})
	if err != nil {
		t.Fatalf("CreateAgentInTeam: %v", err)
	}
	task, err := s.CreateTask(ctx, &coretask.CreateInput{
		TeamID: teamID, AgentID: &agent.ID, Input: "first turn", CreatedBy: userID,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&taskRunRow{}, "task_id = ?", task.ID)
		_ = s.db.WithContext(ctx).Delete(&taskRow{}, "task_id = ?", task.ID)
	})
	firstRunID := *task.LastRunID
	startTaskRunForTest(t, s, ctx, firstRunID)
	if updated, err := s.TransitionTaskRun(ctx, coretask.TransitionRunInput{
		TaskRunID: firstRunID, ExpectedStatus: coretask.RunStatusRunning, NewStatus: coretask.RunStatusSucceeded,
	}); err != nil || !updated {
		t.Fatalf("TransitionTaskRun to SUCCEEDED: updated=%v err=%v", updated, err)
	}

	second, err := s.CreateTaskRun(ctx, coretask.CreateRunInput{
		TaskID: task.ID, Input: "continue with the follow-up", CreatedBy: userID,
	})
	if err != nil {
		t.Fatalf("CreateTaskRun (continue): %v", err)
	}
	if second.TaskID != task.ID {
		t.Errorf("second run TaskID = %q, want %q", second.TaskID, task.ID)
	}
	if second.ID == firstRunID {
		t.Error("continue should create a new run, not reuse the first")
	}
}

// A client that cannot tell whether its Continue request landed retries it
// with the same idempotency key. The second call must return the run the
// first one created — not a second run, and not ErrRunInProgress just because
// the first run is still active.
func TestCreateTaskRunIsIdempotentByKey(t *testing.T) {
	s, ctx := newTestStore(t)
	userID := newTestUser(t, s, "idempotency-key")
	teamID := newTestTeam(t, s, userID)

	agent, err := s.CreateAgentInTeam(ctx, agentdef.CreateInput{
		TeamID: teamID, UserID: userID, Def: agentdef.Definition{Name: "idempotent-runner"},
	})
	if err != nil {
		t.Fatalf("CreateAgentInTeam: %v", err)
	}
	task, err := s.CreateTask(ctx, &coretask.CreateInput{
		TeamID: teamID, AgentID: &agent.ID, Input: "first turn", CreatedBy: userID,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&taskRunRow{}, "task_id = ?", task.ID)
		_ = s.db.WithContext(ctx).Delete(&taskRow{}, "task_id = ?", task.ID)
	})
	firstRunID := *task.LastRunID
	startTaskRunForTest(t, s, ctx, firstRunID)
	if updated, err := s.TransitionTaskRun(ctx, coretask.TransitionRunInput{
		TaskRunID: firstRunID, ExpectedStatus: coretask.RunStatusRunning, NewStatus: coretask.RunStatusSucceeded,
	}); err != nil || !updated {
		t.Fatalf("TransitionTaskRun to SUCCEEDED: updated=%v err=%v", updated, err)
	}

	key := "continue-" + testPublicID(t)
	first, err := s.CreateTaskRun(ctx, coretask.CreateRunInput{
		TaskID: task.ID, Input: "the follow-up", CreatedBy: userID, IdempotencyKey: &key,
	})
	if err != nil {
		t.Fatalf("CreateTaskRun (first Continue): %v", err)
	}
	if first.IdempotencyKey == nil || *first.IdempotencyKey != key {
		t.Errorf("first.IdempotencyKey = %v, want %q", first.IdempotencyKey, key)
	}

	// Retried while the run it created is still active: must return that run,
	// not ErrRunInProgress.
	retry, err := s.CreateTaskRun(ctx, coretask.CreateRunInput{
		TaskID: task.ID, Input: "the follow-up", CreatedBy: userID, IdempotencyKey: &key,
	})
	if err != nil {
		t.Fatalf("CreateTaskRun (retried Continue, run still active): %v", err)
	}
	if retry.ID != first.ID {
		t.Errorf("retried Continue returned run %q, want the original %q", retry.ID, first.ID)
	}

	// Retried again after the run it created has since finished: must still
	// return that same run, not a second one created after it went terminal.
	startTaskRunForTest(t, s, ctx, first.ID)
	if updated, err := s.TransitionTaskRun(ctx, coretask.TransitionRunInput{
		TaskRunID: first.ID, ExpectedStatus: coretask.RunStatusRunning, NewStatus: coretask.RunStatusSucceeded,
	}); err != nil || !updated {
		t.Fatalf("TransitionTaskRun to SUCCEEDED: updated=%v err=%v", updated, err)
	}
	afterFinish, err := s.CreateTaskRun(ctx, coretask.CreateRunInput{
		TaskID: task.ID, Input: "the follow-up", CreatedBy: userID, IdempotencyKey: &key,
	})
	if err != nil {
		t.Fatalf("CreateTaskRun (retried Continue, run finished): %v", err)
	}
	if afterFinish.ID != first.ID {
		t.Errorf("Continue retried after the run finished returned %q, want the original %q", afterFinish.ID, first.ID)
	}

	// A different key on the same task is not a duplicate: it creates its own
	// run, once the task has no active one to refuse it.
	otherKey := "continue-" + testPublicID(t)
	distinct, err := s.CreateTaskRun(ctx, coretask.CreateRunInput{
		TaskID: task.ID, Input: "a different follow-up", CreatedBy: userID, IdempotencyKey: &otherKey,
	})
	if err != nil {
		t.Fatalf("CreateTaskRun (distinct key): %v", err)
	}
	if distinct.ID == first.ID {
		t.Error("a different idempotency key returned the same run")
	}
}
