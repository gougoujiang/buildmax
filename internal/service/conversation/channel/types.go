package channel

import "github.com/gougoujiang/buildmax/internal/core/model"

// Turn is the normalized input from any channel. Adapters produce it from
// channel-specific raw input; the conversation engine consumes it.
type Turn struct {
	Channel        string
	ConversationID string
	UserID         string
	Message        string
	Raw            map[string]any
}

const (
	ChannelPortal   = "portal"
	ChannelTelegram = "telegram"
	ChannelCron     = "cron"
	ChannelWebhook  = "webhook"
	ChannelSystem   = "system"
	// ChannelWorkflow and ChannelIssueAgent are not transports and are defined
	// in core/model with the column they are written to. They are aliased here
	// so a caller that already speaks this package does not need both.
	ChannelWorkflow   = model.ChannelWorkflow
	ChannelIssueAgent = model.ChannelIssueAgent
)

// SyntheticChannels are conversations nobody holds; see core/model.
var SyntheticChannels = model.SyntheticChannels

var ValidChannels = []string{
	ChannelPortal,
	ChannelTelegram,
	ChannelCron,
	ChannelWebhook,
}

func ValidChannel(ch string) bool {
	for _, c := range ValidChannels {
		if ch == c {
			return true
		}
	}
	return false
}
