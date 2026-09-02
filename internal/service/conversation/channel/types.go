package channel

import "context"

// Adapter normalizes channel-specific input into a Turn and can deliver output
// back to that channel.
type Adapter interface {
	Receive(ctx context.Context, raw any) (Turn, error)
	Send(ctx context.Context, target string, output string) error
}

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

// ValidChannels returns the transport channels accepted from a caller.
func ValidChannels() []string {
	return []string{ChannelPortal, ChannelTelegram, ChannelCron, ChannelWebhook}
}

func ValidChannel(ch string) bool {
	for _, c := range ValidChannels() {
		if ch == c {
			return true
		}
	}
	return false
}
