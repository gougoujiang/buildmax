package webhook

import (
	"buildmax/internal/core/conversation"
	"buildmax/internal/infra/db"
)

// Config holds dependencies for the webhook HTTP handler.
type Config struct {
	Adapter           conversation.ChannelAdapter
	Engine            conversation.ConversationEngine
	ConversationStore db.ConversationStore
	KeyStore          db.UserWebhookKeyStore
	MessagePath       string
}
