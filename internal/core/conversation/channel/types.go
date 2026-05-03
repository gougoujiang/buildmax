package channel

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
)

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
