package conversation

import (
	"testing"

	"buildmax/internal/storage/entity"
)

func TestStoredRoleForIncomingTurn(t *testing.T) {
	if got := storedRoleForIncomingTurn("portal"); got != "user" {
		t.Fatalf("storedRoleForIncomingTurn(portal) = %q, want user", got)
	}
	if got := storedRoleForIncomingTurn("system"); got != "system" {
		t.Fatalf("storedRoleForIncomingTurn(system) = %q, want system", got)
	}
}

func TestReplayMessageFromStore_SystemBecomesUserInput(t *testing.T) {
	channel := "system"
	msg := replayMessageFromStore(entity.ConversationMessage{
		Role:    "system",
		Content: "[Task Result] status: succeeded",
		Channel: &channel,
	})
	if msg.Role != "user" {
		t.Fatalf("replayMessageFromStore(system).Role = %q, want user", msg.Role)
	}
	if msg.Content != "[Task Result] status: succeeded" {
		t.Fatalf("replayMessageFromStore(system).Content = %q", msg.Content)
	}
}
