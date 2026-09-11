package db

import (
	"errors"
	"testing"

	coreconv "github.com/gougoujiang/buildmax/internal/core/conversation"
)

// TestAppendMessageFencingRejectsStaleWriter proves the R1 turn-lease fencing
// guarantee at the write path: once a conversation has accepted a message under
// a fencing token, a later write carrying a lower token is refused, so a holder
// that resumed after its lease expired cannot append behind the replica that
// superseded it. See docs/design/server-coordination.md §7.
func TestAppendMessageFencingRejectsStaleWriter(t *testing.T) {
	s, ctx := newTestStore(t)
	user := newTestUser(t, s, "fence")
	space := newTestSpace(t, s, user)
	conv, err := s.CreateConversationInSpace(ctx, space, user, "portal", user)
	if err != nil {
		t.Fatalf("CreateConversationInSpace: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.Where("conversation_id = ?", canonicalPublicID(conv.ID)).Delete(&conversationMessageRow{})
		_ = s.db.Delete(&conversationRow{}, "public_id = ?", canonicalPublicID(conv.ID))
	})

	append := func(fence int64, content string) error {
		_, err := s.AppendMessage(ctx, coreconv.AppendInput{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        content,
			Fence:          fence,
		})
		return err
	}

	// The holder establishes the fence, then re-appends under the same token: a
	// single turn writes many messages and every one carries its lease's token.
	if err := append(10, "first"); err != nil {
		t.Fatalf("append fence 10: %v", err)
	}
	if err := append(10, "same holder"); err != nil {
		t.Fatalf("re-append under held fence: %v", err)
	}

	// A stalled holder whose lease expired and was re-granted resumes with the
	// old token; its write must be refused rather than corrupt the history.
	if err := append(5, "stale"); !errors.Is(err, coreconv.ErrStaleTurnWrite) {
		t.Fatalf("stale append error = %v, want ErrStaleTurnWrite", err)
	}

	// The newer holder advances the fence and writes.
	if err := append(20, "newer holder"); err != nil {
		t.Fatalf("append fence 20: %v", err)
	}

	// An unfenced write (single-instance path) is accepted but must not lower the
	// bar: a fenced write below the advanced token is still refused afterward.
	if err := append(0, "unfenced"); err != nil {
		t.Fatalf("unfenced append: %v", err)
	}
	if err := append(15, "still stale"); !errors.Is(err, coreconv.ErrStaleTurnWrite) {
		t.Fatalf("post-unfenced stale append error = %v, want ErrStaleTurnWrite", err)
	}

	// Only the writes that were accepted are persisted; the two rejected ones are not.
	msgs, err := s.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("persisted %d messages, want 4 (the rejected stale writes must not append)", len(msgs))
	}
}
