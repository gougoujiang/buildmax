package db

import (
	"testing"

	coreconv "github.com/gougoujiang/buildmax/internal/core/conversation"
)

// The filter is in the store because the page is cut there: dropping the rows
// afterwards would leave a short page and a total that disagrees with it.
func TestListConversationsByTeamHidesSyntheticOnes(t *testing.T) {
	s, ctx := newTestStore(t)
	user := newTestUser(t, s, "synthetic-conversations")
	team := newTestTeam(t, s, user)

	mine, err := s.CreateConversationInTeam(ctx, team, user, "portal", user)
	if err != nil {
		t.Fatalf("CreateConversationInTeam: %v", err)
	}
	step, err := s.CreateConversationInTeam(ctx, team, user, coreconv.ChannelWorkflow, user)
	if err != nil {
		t.Fatalf("CreateConversationInTeam workflow: %v", err)
	}
	agentRun, err := s.CreateConversationInTeam(ctx, team, user, coreconv.ChannelIssueAgent, user)
	if err != nil {
		t.Fatalf("CreateConversationInTeam issue agent: %v", err)
	}
	t.Cleanup(func() {
		for _, id := range []string{mine.ID, step.ID, agentRun.ID} {
			_ = s.db.Delete(&conversationRow{}, "conversation_id = ?", id)
		}
	})

	list, total, err := s.ListConversationsByTeam(ctx, team, 50, 0)
	if err != nil {
		t.Fatalf("ListConversationsByTeam: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(list) != 1 || list[0].ID != mine.ID {
		t.Fatalf("list = %+v, want only the conversation someone holds", list)
	}

	// Hidden from the list, not gone: a link straight to one still opens it.
	got, err := s.GetConversation(ctx, step.ID)
	if err != nil || got == nil {
		t.Fatalf("GetConversation on a workflow conversation = %v, %v", got, err)
	}
}
