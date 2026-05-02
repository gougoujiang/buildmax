package portalapi

import (
	"context"

	"buildmax/internal/core/quota"
	"buildmax/internal/infra/db"
	llm "buildmax/internal/infra/llm"
	blob "buildmax/internal/infra/objectstore"
	streamhub "buildmax/internal/server/websocket"
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
	ListRunOutputsByConversation(ctx context.Context, conversationID string, taskID *string) ([]db.ArtifactWithTask, error)
	GetTaskRunOutputFiles(ctx context.Context, taskRunID string) ([]db.TaskRunArtifact, error)
}

// Config holds dependencies for authenticated portal endpoints.
type Config struct {
	JWTSecret                string
	CORSOrigin               string // Allowed origin for WebSocket upgrade check
	UserStore                db.UserStore
	TeamStore                db.TeamStore
	WorkflowStore            db.WorkflowStore
	AgentStore               db.AgentStore
	IssueStore               db.IssueStore
	TaskStore                db.TaskStore
	TaskRunStore             db.TaskRunStore
	RunOutputLister          RunOutputLister
	PersistStorage           blob.PersistStorage
	ArtifactStorage          blob.ArtifactStorage
	WorkspacesDir            string
	DefaultQuotaTier         string
	QuotaChecker             *quota.Checker
	TitleGenerator           TitleGenerator
	ConversationStore        db.ConversationStore
	ConversationMessageStore db.ConversationMessageStore
	ConversationLLMCaller    llm.LLMCaller
	Hub                      streamhub.StreamHub
	UserWebhookKeyStore      db.UserWebhookKeyStore
	ConnRegistry             *ConnRegistry
}
