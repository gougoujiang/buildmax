package webhook

import (
	"buildmax/internal/conversation"
	"buildmax/internal/storage/entity"
)

// Config holds dependencies for the webhook HTTP handler.
type Config struct {
	Adapter     conversation.ChannelAdapter
	Engine      conversation.ConversationEngine
	WorkspaceStore entity.WorkspaceStore
	KeyStore    entity.WorkspaceWebhookKeyStore
	MessagePath string
}
