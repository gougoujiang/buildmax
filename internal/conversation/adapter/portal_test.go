package adapter

import (
	"context"
	"testing"

	"buildmax/internal/conversation"
)

func TestPortalAdapter_Receive(t *testing.T) {
	ctx := context.Background()
	adapter := PortalAdapter{}

	t.Run("success", func(t *testing.T) {
		input := &PortalTurnInput{
			WorkspaceID: "w_ws1",
			ChatID:      "c_chat1",
			UserID:      "u_user1",
			Message:     "hello",
		}
		turn, err := adapter.Receive(ctx, input)
		if err != nil {
			t.Fatalf("Receive: %v", err)
		}
		if turn.Channel != conversation.ChannelPortal {
			t.Errorf("Channel = %q, want %q", turn.Channel, conversation.ChannelPortal)
		}
		if turn.ConversationID != input.ChatID {
			t.Errorf("ConversationID = %q, want %q", turn.ConversationID, input.ChatID)
		}
		if turn.UserID != input.UserID {
			t.Errorf("UserID = %q, want %q", turn.UserID, input.UserID)
		}
		if turn.Message != input.Message {
			t.Errorf("Message = %q, want %q", turn.Message, input.Message)
		}
		if turn.WorkspaceID != input.WorkspaceID {
			t.Errorf("WorkspaceID = %q, want %q", turn.WorkspaceID, input.WorkspaceID)
		}
	})

	t.Run("nil raw returns error", func(t *testing.T) {
		_, err := adapter.Receive(ctx, nil)
		if err == nil {
			t.Error("Receive(nil): expected error")
		}
	})

	t.Run("wrong type returns error", func(t *testing.T) {
		_, err := adapter.Receive(ctx, "not a pointer")
		if err == nil {
			t.Error("Receive(wrong type): expected error")
		}
	})
}

func TestPortalAdapter_Send(t *testing.T) {
	ctx := context.Background()
	adapter := PortalAdapter{}
	if err := adapter.Send(ctx, "c_1", "output"); err != nil {
		t.Errorf("Send: %v", err)
	}
}
