package conversation

import (
	"context"

	convruntime "buildmax/internal/core/conversation/runtime"
	"buildmax/internal/core/model"
)

type TurnToolRunners = convruntime.TurnToolRunners

type TurnRunInput = convruntime.TurnRunInput

// DefaultConversationTools returns the default tool set for the conversation loop (GetCurrentDate only).
func DefaultConversationTools() []model.Tool {
	return convruntime.DefaultConversationTools()
}

// Run executes one conversation turn. Streaming is enabled when in.StreamSink is non-nil.
func Run(ctx context.Context, convStore model.ConversationStore, msgStore model.ConversationMessageStore, llmClient model.LLMClient, in TurnRunInput) (string, error) {
	return convruntime.Run(ctx, convStore, msgStore, llmClient, in)
}

// RunLoop loads conversation messages, appends the new user message, runs the LLM loop with the given
// tools, and persists every assistant and tool message to the store. Returns the final assistant text reply.
// If toolsList is nil, the runtime builds the default conversation tools plus any tool runners provided here.
// recentTasksSnippet, when non-empty, is appended to the system prompt (e.g. latest 5 tasks).
// If titleGenerator is non-nil and this is the first round (no messages before), a title is generated from userContent and saved.
func RunLoop(
	ctx context.Context,
	convStore model.ConversationStore,
	msgStore model.ConversationMessageStore,
	llmClient model.LLMClient,
	conversationID string,
	userContent string,
	channel string,
	toolsList []model.Tool,
	scopeID, userID string,
	runners *TurnToolRunners,
	titleGenerator model.TitleGenerator,
	recentTasksSnippet string,
) (reply string, err error) {
	return Run(ctx, convStore, msgStore, llmClient, TurnRunInput{
		ConversationID:      conversationID,
		Message:             userContent,
		Channel:             channel,
		ToolsList:           toolsList,
		ConversationScopeID: scopeID,
		UserID:              userID,
		Runners:             runners,
		TitleGenerator:      titleGenerator,
		RecentTasksSnippet:  recentTasksSnippet,
	})
}

// RunLoopStream is like RunLoop but streams assistant content deltas via sink.
// When the model returns tool calls, those turns are not streamed; only the final (or intermediate) text content is streamed.
// If toolsList is nil, the runtime builds the default conversation tools plus any tool runners provided here.
// recentTasksSnippet, when non-empty, is appended to the system prompt.
// If titleGenerator is non-nil and this is the first round, a title is generated from userContent and saved.
func RunLoopStream(
	ctx context.Context,
	convStore model.ConversationStore,
	msgStore model.ConversationMessageStore,
	llmClient model.LLMClient,
	conversationID string,
	userContent string,
	channel string,
	toolsList []model.Tool,
	scopeID, userID string,
	runners *TurnToolRunners,
	titleGenerator model.TitleGenerator,
	sink model.StreamSink,
	recentTasksSnippet string,
) (reply string, err error) {
	return Run(ctx, convStore, msgStore, llmClient, TurnRunInput{
		ConversationID:      conversationID,
		Message:             userContent,
		Channel:             channel,
		ToolsList:           toolsList,
		ConversationScopeID: scopeID,
		UserID:              userID,
		Runners:             runners,
		TitleGenerator:      titleGenerator,
		RecentTasksSnippet:  recentTasksSnippet,
		StreamSink:          sink,
	})
}
