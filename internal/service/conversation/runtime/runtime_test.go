package runtime

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

func TestReplayMessageFromStore_SystemBecomesUser_BackwardCompat(t *testing.T) {
	channel := "system"
	msg := replayMessageFromStore(model.ConversationMessage{
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

func TestReplayMessageFromStore_UserRolePassthrough(t *testing.T) {
	channel := "portal"
	msg := replayMessageFromStore(model.ConversationMessage{
		Role:    "user",
		Content: "hello",
		Channel: &channel,
	})
	if msg.Role != "user" {
		t.Fatalf("replayMessageFromStore(user).Role = %q, want user", msg.Role)
	}
}
