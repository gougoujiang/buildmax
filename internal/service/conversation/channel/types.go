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
	ChannelWebhook  = "webhook"
	ChannelSystem   = "system"
)

// ValidChannels returns the transport channels accepted from a caller.
//
// There is no cron channel: a recurring schedule is not a conversation
// transport. It runs an Agent directly on the Task plane through a schedule
// trigger source, not by delivering a turn to a conversation. See
// docs/proposals/scheduled-agent-execution.md §11.
func ValidChannels() []string {
	return []string{ChannelPortal, ChannelTelegram, ChannelWebhook}
}

func ValidChannel(ch string) bool {
	for _, c := range ValidChannels() {
		if ch == c {
			return true
		}
	}
	return false
}
