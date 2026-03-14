// Interfaces for Tier 1 channel adapters and conversation engines.
// See design/007-two-tier-agent.md.

package conversation

import "context"

// ChannelAdapter normalizes channel-specific input into a ConversationTurn
// and can deliver output (e.g. reply or run result) back to the channel.
type ChannelAdapter interface {
	// Receive normalizes raw input (e.g. HTTP request, config) into a ConversationTurn.
	Receive(ctx context.Context, raw any) (ConversationTurn, error)
	// Send delivers output to the channel (e.g. POST to callback URL).
	// conversationID is channel-specific (e.g. callback URL for webhook).
	Send(ctx context.Context, conversationID string, output string) error
}

// ConversationEngine processes one turn; may create Tier 2 runs and/or return a direct reply.
type ConversationEngine interface {
	// Process handles a normalized turn. workspaceID and chatID identify the target;
	// chatID may be empty to create a new chat. Returns reply and/or task IDs (chat_run_ids).
	Process(ctx context.Context, workspaceID, chatID string, turn ConversationTurn) (ConversationResult, error)
}
