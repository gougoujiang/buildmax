package handlers

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/server/handlers/admin"
	"github.com/gougoujiang/buildmax/internal/server/turnqueue"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	wsconn "github.com/gougoujiang/buildmax/internal/server/websocket"
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

// Config holds all dependencies for the unified handler (auth, user API, worker API, inbound webhook).
type Config struct {
	JWTSecret   string
	CORSOrigin  string
	WorkerToken string // required for /api/worker/* endpoints
	// WorkerLLM tells a worker how to reach a model for its run. Nil means
	// direct, which is what a deployment that has not enabled managed worker
	// inference reports.
	WorkerLLM *workerclient.TaskRunLLM
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
	UserStore         model.UserStore
	LoginCodeStore    model.LoginCodeStore
	PasswordStore     model.PasswordStore
	RefreshTokenStore model.RefreshTokenStore
	TeamStore         model.TeamStore
	WorkflowStore     model.WorkflowStore
	AgentStore        model.AgentStore
	IssueStore        model.IssueStore
	IssueCommentStore model.IssueCommentStore
	TaskStore         model.TaskStore
	TaskRunStore      model.TaskRunStore
	// LLMCallStore reads the managed call ledger. Nil leaves the ledger
	// unreadable over HTTP, which is what a deployment with no database has.
	LLMCallStore             model.LLMCallStore
	RunOutputLister          RunOutputLister
	UserWebhookKeyStore      model.UserWebhookKeyStore
	ConversationStore        model.ConversationStore
	ConversationMessageStore model.ConversationMessageStore
	AuditStore               model.AuditStore
	// LLMModelStore reads the managed model catalog. Nil leaves the admin
	// catalog routes answering 503. It is deliberately read plus enable/disable
	// here: the credential half of the catalog is edited by
	// `buildmax-server model`, on the machine that holds the database
	// credentials.
	LLMModelStore model.LLMModelStore
	// SchemaStore reports which migrations a database has had applied. Nil
	// leaves that field of the system status empty.
	SchemaStore model.SchemaStore
	// ArtifactStore records durable files. Nil, or no ArtifactStorage, leaves
	// the artifact routes answering 503: metadata without content is not an
	// artifact capability.
	ArtifactStore model.ArtifactStore
	// SystemGrantStore reads deployment-scoped role grants. Nil leaves every
	// /api/admin route answering 503 to an authenticated caller, which is what
	// a deployment with no database has: no way to know whether anyone is an
	// administrator, and therefore no basis for letting one in.
	SystemGrantStore model.SystemGrantStore

	// Storage
	PersistStorage   blob.PersistStorage
	RunOutputStorage blob.RunOutputStorage
	ArtifactStorage  blob.ArtifactStorage
	MaxArtifactBytes int64
	WorkspacesDir    string

	// Auth / quota
	DefaultQuotaTier string
	QuotaService     *quota.Service

	// Conversation / LLM
	TitleGenerator        llm.TitleGenerator
	ConversationLLMClient llm.LLMClient

	// LLMGateway serves managed inference. Nil means the deployment offers no
	// managed models and the /llm routes answer 503.
	LLMGateway *llmgateway.Service

	// Audit records sensitive actions. Nil discards them, so a deployment
	// without a database still serves.
	Audit *audit.Recorder

	// Deployment describes facts about this deployment that do not change
	// while it runs, for the admin system status.
	Deployment admin.DeploymentInfo
	// DependencyProbes are the same checks the readiness endpoint runs. The
	// admin status reports them so an operator sees what /readyz sees without
	// needing to reach it.
	DependencyProbes []admin.DependencyProbe
	// RedactedConfig is the operator-facing view of server.yaml, built by
	// internal/config so that the decision about which fields may be shown
	// lives next to the struct. Nil means the deployment reports none.
	RedactedConfig any

	// Inbound webhook
	WebhookAdapter     convchannel.Adapter
	WebhookEngine      conversation.TurnEngine
	WebhookMessagePath string

	// Hub is optional; if nil NewHandler creates one. Injectable for testing.
	Hub wsconn.StreamHub

	// OnTaskRunTerminal is an optional external callback fired when a worker run reaches
	// terminal status (after the internal hub/registry callbacks run).
	OnTaskRunTerminal func(ctx context.Context, info model.TaskRunTerminalInfo)
}

// Handler serves all HTTP routes: auth, user API, worker API, inbound webhook.
type Handler struct {
	cfg          Config
	hub          wsconn.StreamHub
	connRegistry *wsconn.ConnRegistry
	// turns serializes the turns of one conversation and queues the rest. It is
	// server-scoped, not connection-scoped — see turnqueue.Registry.
	turns *turnqueue.Registry
}

// NewHandler returns a configured Handler. If cfg.Hub is nil a new StreamHub is created internally.
func NewHandler(cfg Config) *Handler {
	hub := cfg.Hub
	if hub == nil {
		hub = wsconn.NewStreamHub()
	}
	return &Handler{
		cfg:          cfg,
		hub:          hub,
		connRegistry: wsconn.NewConnRegistry(),
		turns:        turnqueue.NewRegistry(),
	}
}
