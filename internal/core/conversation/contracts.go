package conversation

import (
	"context"

	convchannel "buildmax/internal/core/conversation/channel"
)

type ConversationTurn = convchannel.Turn

// ConversationResult is the output of processing one turn. The engine may return
// a direct reply and/or one or more spawned task_run_ids.
type ConversationResult struct {
	// Reply is an optional direct reply to the user (e.g. clarification or acknowledgment).
	Reply string
	// TaskRunIDs are task_run_ids of Tier 2 runs spawned for this turn (if any).
	TaskRunIDs []string
}

const (
	ChannelPortal   = convchannel.ChannelPortal
	ChannelTelegram = convchannel.ChannelTelegram
	ChannelCron     = convchannel.ChannelCron
	ChannelWebhook  = convchannel.ChannelWebhook
	ChannelSystem   = convchannel.ChannelSystem
)

var ValidChannels = convchannel.ValidChannels

func ValidChannel(ch string) bool {
	return convchannel.ValidChannel(ch)
}

type ChannelAdapter = convchannel.Adapter

type WebhookRequest = convchannel.WebhookRequest

type WebhookAdapter = convchannel.WebhookAdapter

const DefaultWebhookUserID = convchannel.DefaultWebhookUserID

func NewWebhookAdapter(messagePath string, userID string) *WebhookAdapter {
	return convchannel.NewWebhookAdapter(messagePath, userID)
}

// TurnEngine processes one turn; may create Tier 2 runs and/or return a direct reply.
type TurnEngine interface {
	Process(ctx context.Context, conversationID, taskID string, turn ConversationTurn) (ConversationResult, error)
}
