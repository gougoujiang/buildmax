package webhook

import (
	"buildmax/internal/core/conversation"
	"buildmax/internal/core/model"
)

// Config holds dependencies for the webhook HTTP handler.
type Config struct {
	Adapter           conversation.ChannelAdapter
	Engine            conversation.ConversationEngine
	ConversationStore model.ConversationStore
	KeyStore          model.UserWebhookKeyStore
	MessagePath       string
}
