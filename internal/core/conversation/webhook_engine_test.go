package conversation

import (
	"context"
	"errors"
	"testing"

	convchannel "buildmax/internal/core/conversation/channel"
	"buildmax/internal/core/task"
	"buildmax/internal/mock"
)

func TestWebhookEngine_Process_wrongChannel(t *testing.T) {
	e := &WebhookEngine{}
	ctx := context.Background()
	turn := convchannel.Turn{
		Channel: ChannelPortal,
		Message: "hi",
		UserID:  "u_1",
	}
	_, err := e.Process(ctx, "w_1", "", turn)
	if err == nil {
		t.Fatal("expected error for non-webhook channel")
	}
	if !errors.Is(err, ErrWebhookChannelRequired) {
		t.Errorf("err = %v, want ErrWebhookChannelRequired", err)
	}
}

func TestWebhookEngine_Process_chatNil(t *testing.T) {
	e := &WebhookEngine{TaskService: nil}
	ctx := context.Background()
	turn := convchannel.Turn{
		Channel: ChannelWebhook,
		Message: "task",
		UserID:  "webhook",
	}
	_, err := e.Process(ctx, "w_1", "", turn)
	if err == nil {
		t.Fatal("expected error when TaskService is nil")
	}
	if !errors.Is(err, task.ErrTaskRunsNotConfigured) {
		t.Errorf("err = %v", err)
	}
}

func TestWebhookEngine_Process_webhookEmptyMessage(t *testing.T) {
	e := &WebhookEngine{
		TaskService:   &task.TaskService{},
		Conversations: &mock.MockConversationStore{},
	} // Task service has nil stores; CreateTask will fail later, but we validate message first
	ctx := context.Background()
	turn := convchannel.Turn{
		Channel: ChannelWebhook,
		Message: "",
		UserID:  "webhook",
	}
	_, err := e.Process(ctx, "w_1", "", turn)
	if err == nil {
		t.Fatal("expected error for empty message")
	}
}

func TestWebhookEngine_Process_requiresConversationStore(t *testing.T) {
	e := &WebhookEngine{TaskService: &task.TaskService{}}
	ctx := context.Background()
	turn := convchannel.Turn{
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
