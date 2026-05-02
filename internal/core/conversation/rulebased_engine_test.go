package conversation

import (
	"context"
	"errors"
	"testing"

	taskapp "buildmax/internal/core/task"
	"buildmax/internal/mock"
)

func TestRuleBasedEngine_Process_wrongChannel(t *testing.T) {
	e := &RuleBasedEngine{}
	ctx := context.Background()
	turn := ConversationTurn{
		Channel: ChannelPortal,
		Message: "hi",
		UserID:  "u_1",
	}
	_, err := e.Process(ctx, "w_1", "", turn)
	if err == nil {
		t.Fatal("expected error for non-webhook channel")
	}
	if !errors.Is(err, ErrChannelNotWebhook) {
		t.Errorf("err = %v, want ErrChannelNotWebhook", err)
	}
}

func TestRuleBasedEngine_Process_chatNil(t *testing.T) {
	e := &RuleBasedEngine{Task: nil}
	ctx := context.Background()
	turn := ConversationTurn{
		Channel: ChannelWebhook,
		Message: "task",
		UserID:  "webhook",
	}
	_, err := e.Process(ctx, "w_1", "", turn)
	if err == nil {
		t.Fatal("expected error when Task is nil")
	}
	if !errors.Is(err, taskapp.ErrTaskRunsNotConfigured) {
		t.Errorf("err = %v", err)
	}
}

func TestRuleBasedEngine_Process_webhookEmptyMessage(t *testing.T) {
	e := &RuleBasedEngine{
		Task:          &taskapp.Service{},
		Conversations: &mock.MockConversationStore{},
	} // Task service has nil stores; CreateTask will fail later, but we validate message first
	ctx := context.Background()
	turn := ConversationTurn{
		Channel: ChannelWebhook,
		Message: "",
		UserID:  "webhook",
	}
	_, err := e.Process(ctx, "w_1", "", turn)
	if err == nil {
		t.Fatal("expected error for empty message")
	}
}

func TestRuleBasedEngine_Process_requiresConversationStore(t *testing.T) {
	e := &RuleBasedEngine{Task: &taskapp.Service{}}
	ctx := context.Background()
	turn := ConversationTurn{
		Channel: ChannelWebhook,
		Message: "task",
		UserID:  "webhook",
	}
	_, err := e.Process(ctx, "w_1", "", turn)
	if err == nil {
		t.Fatal("expected error when Conversations is nil")
	}
	if err.Error() != "conversation store not configured" {
		t.Fatalf("err = %v, want conversation store not configured", err)
	}
}
