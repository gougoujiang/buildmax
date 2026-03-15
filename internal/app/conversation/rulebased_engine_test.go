package conversation

import (
	"context"
	"errors"
	"testing"

	taskapp "buildmax/internal/app/task"
	coreconv "buildmax/internal/conversation"
)

func TestRuleBasedEngine_Process_wrongChannel(t *testing.T) {
	e := &RuleBasedEngine{}
	ctx := context.Background()
	turn := coreconv.ConversationTurn{
		Channel: coreconv.ChannelPortal,
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
	turn := coreconv.ConversationTurn{
		Channel: coreconv.ChannelWebhook,
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
	e := &RuleBasedEngine{Task: &taskapp.Service{}} // Task service has nil stores; CreateTask will fail later, but we validate message first
	ctx := context.Background()
	turn := coreconv.ConversationTurn{
		Channel: coreconv.ChannelWebhook,
		Message: "",
		UserID:  "webhook",
	}
	_, err := e.Process(ctx, "w_1", "", turn)
	if err == nil {
		t.Fatal("expected error for empty message")
	}
}
