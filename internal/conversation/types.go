// Package conversation defines the Tier 1 conversation layer contract: normalized
// turn/result types, channel names, and interfaces for channel adapters and
// conversation engines. See design/007-two-tier-agent.md.
package conversation

// ConversationTurn is the normalized input from any channel. Adapters produce
// it from channel-specific raw input; the conversation engine consumes it.
type ConversationTurn struct {
	// Channel is which channel the input came from (e.g. ChannelPortal, ChannelTelegram).
	Channel string
	// ConversationID is the channel-specific conversation or thread id (e.g. chat_id for portal).
	ConversationID string
	// UserID is the user who sent the message or triggered the turn.
	UserID string
	// Message is the user-facing message or task input text.
	Message string
	// Raw holds optional channel-specific payload for adapters that need extra context.
	Raw map[string]any
}

// ConversationResult is the output of processing one turn. The engine may return
// a direct reply and/or one or more task_run_ids (TaskIDs) that were spawned.
type ConversationResult struct {
	// Reply is an optional direct reply to the user (e.g. clarification or acknowledgment).
	Reply string
	// TaskIDs are task_run_ids of Tier 2 runs spawned for this turn (if any).
	TaskIDs []string
}

// Channel name constants. Use these instead of magic strings so adapters and
// engines refer to the same channel identifiers.
const (
	ChannelPortal   = "portal"
	ChannelTelegram = "telegram"
	ChannelCron     = "cron"
	ChannelWebhook  = "webhook"
)

// ValidChannels is the list of all valid channel names. Useful for validation
// or iteration.
var ValidChannels = []string{
	ChannelPortal,
	ChannelTelegram,
	ChannelCron,
	ChannelWebhook,
}

// ValidChannel returns true if ch is one of the four channel constants.
func ValidChannel(ch string) bool {
	for _, c := range ValidChannels {
		if ch == c {
			return true
		}
	}
	return false
}
