package db

import "testing"

func TestListConversationsByTeamReturnsTeamConversations(t *testing.T) {
	s, ctx := newTestStore(t)
	user := newTestUser(t, s, "team-conversations")
	team := newTestTeam(t, s, user)

	mine, err := s.CreateConversationInTeam(ctx, team, user, "portal", user)
	if err != nil {
		t.Fatalf("CreateConversationInTeam: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.Delete(&conversationRow{}, "conversation_id = ?", mine.ID)
	})

	list, total, err := s.ListConversationsByTeam(ctx, team, 50, 0)
	if err != nil {
		t.Fatalf("ListConversationsByTeam: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(list) != 1 || list[0].ID != mine.ID {
		t.Fatalf("list = %+v, want only %s", list, mine.ID)
	}
}
