package portalapi

import (
	"context"

	"buildmax/internal/core/model"
	"buildmax/internal/core/quota"
	blob "buildmax/internal/infra/objectstore"
	streamhub "buildmax/internal/server/websocket"
)

// RunOutputLister lists run outputs by conversation and gets output files for a run.
type RunOutputLister interface {
	ListRunOutputsByConversation(ctx context.Context, conversationID string, taskID *string) ([]model.ArtifactWithTask, error)
	GetTaskRunOutputFiles(ctx context.Context, taskRunID string) ([]model.TaskRunArtifact, error)
}

// Config holds dependencies for authenticated portal endpoints.
type Config struct {
	JWTSecret                string
	CORSOrigin               string // Allowed origin for WebSocket upgrade check
	UserStore                model.UserStore
	TeamStore                model.TeamStore
	WorkflowStore            model.WorkflowStore
	AgentStore               model.AgentStore
	IssueStore               model.IssueStore
	TaskStore                model.TaskStore
	TaskRunStore             model.TaskRunStore
	RunOutputLister          RunOutputLister
	PersistStorage           blob.PersistStorage
	ArtifactStorage          blob.ArtifactStorage
	WorkspacesDir            string
	DefaultQuotaTier         string
	QuotaChecker             *quota.Checker
	TitleGenerator           model.TitleGenerator
	ConversationStore        model.ConversationStore
	ConversationMessageStore model.ConversationMessageStore
	ConversationLLMClient    model.LLMClient
	Hub                      streamhub.StreamHub
	UserWebhookKeyStore      model.UserWebhookKeyStore
	ConnRegistry             *ConnRegistry
}
