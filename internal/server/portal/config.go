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

// TitleGenerator generates a short title from prompt input. Optional.
type TitleGenerator interface {
	GenerateTitle(ctx context.Context, input string) (title string, usage TokenUsage, err error)
}

// RunOutputLister lists run outputs by conversation and gets output files for a run.
type RunOutputLister interface {
	ListRunOutputsByConversation(ctx context.Context, conversationID string, taskID *string) ([]entity.ArtifactWithTask, error)
	GetTaskRunOutputFiles(ctx context.Context, taskRunID string) ([]entity.TaskRunArtifact, error)
}

// Config holds dependencies for authenticated portal endpoints.
type Config struct {
	JWTSecret                string
	CORSOrigin               string // Allowed origin for WebSocket upgrade check
	AgentStore               entity.AgentStore
	IssueStore               entity.IssueStore
	TaskStore                entity.TaskStore
	TaskRunStore             entity.TaskRunStore
	RunOutputLister          RunOutputLister
	PersistStorage           blob.PersistStorage
	ArtifactStorage          blob.ArtifactStorage
	WorkspacesDir            string
	QuotaChecker             *quota.Checker
	TitleGenerator           TitleGenerator
	ConversationStore        entity.ConversationStore
	ConversationMessageStore entity.ConversationMessageStore
	ConversationLLMCaller    llm.LLMCaller
	Hub                      streamhub.StreamHub
	UserWebhookKeyStore      entity.UserWebhookKeyStore
	ConnRegistry             *ConnRegistry
}
