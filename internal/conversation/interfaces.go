package conversation

import "context"

// ChannelAdapter normalizes channel-specific input into ConversationTurns and
// delivers output back to the channel. One implementation per channel type
// (portal HTTP, Telegram, cron, webhook).
type ChannelAdapter interface {
	// Receive normalizes raw input (e.g. HTTP request body, webhook payload) into
	// a ConversationTurn. The type of raw is channel-specific.
	Receive(ctx context.Context, raw any) (ConversationTurn, error)
	// Send delivers output (reply or task result) to the channel. conversationID
	// is the same ConversationID from the turn (e.g. chat_id for portal).
	Send(ctx context.Context, conversationID string, output string) error
}

// ConversationEngine processes one conversation turn. It may respond only, ask
// for clarification, or spawn one or more Tier 2 tasks (chat runs). Implementations
// include pass-through (always spawn one run) and LLM-based (decide reply vs spawn).
type ConversationEngine interface {
	// Process handles one turn for the given workspace and chat. Returns a
	// ConversationResult with an optional Reply and/or TaskIDs (chat_run_ids).
	Process(ctx context.Context, workspaceID, chatID string, turn ConversationTurn) (ConversationResult, error)
}
