package work

// Fixtures copied rather than imported: a helper crossing a package boundary
// makes the test boundary softer than the code's.

import (
	"context"
	"time"

	"github.com/gougoujiang/buildmax/internal/mock"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// llmStubLedger accepts every write and keeps the last one so a test can check
// what a call was attributed to.
type llmStubLedger struct {
	opened  int
	last    model.LLMCall
	calls   []model.LLMCall
	listErr error
}

func (l *llmStubLedger) OpenLLMCall(_ context.Context, call *model.LLMCall) (*model.LLMCall, error) {
	l.opened++
	stored := *call
	stored.ID = "lc_stub"
	l.last = stored
	return &stored, nil
}
func (l *llmStubLedger) CompleteLLMCall(context.Context, string, model.LLMCallOutcome) error {
	return nil
}
func (l *llmStubLedger) GetLLMCall(context.Context, string) (*model.LLMCall, error) { return nil, nil }

func (l *llmStubLedger) GetLLMCallByClientID(context.Context, string, string) (*model.LLMCall, error) {
	return nil, nil
}
func (l *llmStubLedger) ListLLMCallsByTaskRun(_ context.Context, teamID, taskRunID string) ([]model.LLMCall, error) {
	if l.listErr != nil {
		return nil, l.listErr
	}
	var out []model.LLMCall
	for _, call := range l.calls {
		if call.TeamID == teamID && call.TaskRunID != nil && *call.TaskRunID == taskRunID {
			out = append(out, call)
		}
	}
	return out, nil
}

const llmTestSecret = "test-llm-secret"

func llmTestTeamStore() *mock.MockTeamStore {
	return &mock.MockTeamStore{
		Teams: []model.Team{
			{ID: llmTestTeam, Name: "LLM Team", CreatedBy: llmTestUser, CreatedAt: time.Now().UTC()},
		},
		Members: []model.TeamMember{
			{TeamID: llmTestTeam, UserID: llmTestUser, Role: model.TeamRoleOwner, CreatedAt: time.Now().UTC()},
		},
	}
}

const llmTestUser = "u_llm"

const llmTestTeam = "tm_llm"
