package handlers

import (
	"context"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	streamhub "github.com/gougoujiang/buildmax/internal/server/websocket"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
	convchannel "github.com/gougoujiang/buildmax/internal/service/conversation/channel"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
	"github.com/gougoujiang/buildmax/internal/service/quota"
)

// RunOutputLister lists run outputs (artifacts) by conversation and gets output files for a run.
type RunOutputLister interface {
	ListRunOutputsByConversation(ctx context.Context, conversationID string, taskID *string) ([]model.ArtifactWithTask, error)
	GetTaskRunOutputFiles(ctx context.Context, taskRunID string) ([]model.TaskRunArtifact, error)
}

// TaskRunTerminalInfo holds information about a task run that reached terminal status.
type TaskRunTerminalInfo struct {
	TaskRunID      string
	TaskID         string
	ConversationID string
	UserID         string
	Status         string  // "succeeded" or "failed"
	Output         *string // task output (succeeded) — may be nil
	ErrorMessage   *string // error message (failed) — may be nil
}

// Config holds all dependencies for the unified handler (auth, user API, worker API, inbound webhook).
type Config struct {
	JWTSecret   string
	CORSOrigin  string
	WorkerToken string // required for /api/worker/* endpoints
	// AllowSignup opens POST /api/otp/request to self-registration. False — the
	// zero value — means accounts are created by an operator.
	AllowSignup bool

	// Token lifetimes. Zero means the model package's default. The access
	// token is signed and unstored, so its lifetime is the window in which a
	// stolen one still works; the refresh token is a row and can be revoked
	// before it expires.
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	// RefreshRotationGrace is how long a just-rotated refresh token may be
	// exchanged again before that counts as reuse. It exists because the CLI
	// and Desktop share one credentials file between processes.
	RefreshRotationGrace time.Duration

	// Stores
	UserStore                model.UserStore
	LoginCodeStore           model.LoginCodeStore
	PasswordStore            model.PasswordStore
	RefreshTokenStore        model.RefreshTokenStore
	TeamStore                model.TeamStore
	WorkflowStore            model.WorkflowStore
	AgentStore               model.AgentStore
	IssueStore               model.IssueStore
	TaskStore                model.TaskStore
	TaskRunStore             model.TaskRunStore
	RunOutputLister          RunOutputLister
	UserWebhookKeyStore      model.UserWebhookKeyStore
	ConversationStore        model.ConversationStore
	ConversationMessageStore model.ConversationMessageStore
	AuditStore               model.AuditStore

	// Storage
	PersistStorage  blob.PersistStorage
	ArtifactStorage blob.ArtifactStorage
	WorkspacesDir   string

	// Auth / quota
	DefaultQuotaTier string
	QuotaService     *quota.QuotaService

	// Conversation / LLM
	TitleGenerator        llm.TitleGenerator
	ConversationLLMClient llm.LLMClient

	// LLMGateway serves managed inference. Nil means the deployment offers no
	// managed models and the /llm routes answer 503.
	LLMGateway *llmgateway.Service

	// Audit records sensitive actions. Nil discards them, so a deployment
	// without a database still serves.
	Audit *audit.Recorder

	// Inbound webhook
	WebhookAdapter     convchannel.Adapter
	WebhookEngine      conversation.TurnEngine
	WebhookMessagePath string

	// Hub is optional; if nil NewHandler creates one. Injectable for testing.
	Hub streamhub.StreamHub

	// OnTaskRunTerminal is an optional external callback fired when a worker run reaches
	// terminal status (after the internal hub/registry callbacks run).
	OnTaskRunTerminal func(ctx context.Context, info TaskRunTerminalInfo)
}

// Handler serves all HTTP routes: auth, user API, worker API, inbound webhook.
type Handler struct {
	cfg          Config
	hub          streamhub.StreamHub
	connRegistry *connRegistry
}

// NewHandler returns a configured Handler. If cfg.Hub is nil a new StreamHub is created internally.
func NewHandler(cfg Config) *Handler {
	hub := cfg.Hub
	if hub == nil {
		hub = streamhub.NewStreamHub()
	}
	return &Handler{
		cfg:          cfg,
		hub:          hub,
		connRegistry: newConnRegistry(),
	}
}
