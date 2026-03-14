package portal

import (
	"context"

	"buildmax/internal/llm"
	"buildmax/internal/quota"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
	"buildmax/internal/streamhub"
)

// TokenUsage holds prompt and completion token counts for a single LLM call.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
}

// ChatTitleGenerator generates a short title from chat input. Optional.
type ChatTitleGenerator interface {
	GenerateChatTitle(ctx context.Context, input string) (title string, usage TokenUsage, err error)
}

// RunOutputLister lists run outputs by workspace and gets output files for a run.
type RunOutputLister interface {
	ListRunOutputsByWorkspace(ctx context.Context, workspaceID string, chatID *string) ([]entity.ArtifactWithChat, error)
	GetChatRunOutputFiles(ctx context.Context, chatRunID string) ([]entity.ChatRunArtifact, error)
}

// Config holds dependencies for authenticated portal endpoints.
type Config struct {
	JWTSecret                string
	WorkspaceStore           entity.WorkspaceStore
	AgentStore               entity.AgentStore
	ChatStore                entity.ChatStore
	ChatRunStore             entity.ChatRunStore
	RunOutputLister          RunOutputLister
	PersistStorage           blob.PersistStorage
	ArtifactStorage          blob.ArtifactStorage
	WorkspacesDir            string
	QuotaChecker             *quota.Checker
	ChatTitleGenerator       ChatTitleGenerator
	ConversationStore        entity.ConversationStore
	ConversationMessageStore entity.ConversationMessageStore
	ConversationLLMCaller    llm.LLMCaller
	Hub                      streamhub.StreamHub
}
