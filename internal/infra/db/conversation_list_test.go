package db

import "testing"

func TestListConversationsBySpaceReturnsSpaceConversations(t *testing.T) {
	s, ctx := newTestStore(t)
	user := newTestUser(t, s, "space-conversations")
	space := newTestSpace(t, s, user)

	mine, err := s.CreateConversationInSpace(ctx, space, user, "portal", user)
	if err != nil {
		t.Fatalf("CreateConversationInSpace: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.Delete(&conversationRow{}, "conversation_id = ?", mine.ID)
	})

	list, total, err := s.ListConversationsBySpace(ctx, space, 50, 0)
	if err != nil {
		t.Fatalf("ListConversationsBySpace: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(list) != 1 || list[0].ID != mine.ID {
		t.Fatalf("list = %+v, want only %s", list, mine.ID)
	}
}
