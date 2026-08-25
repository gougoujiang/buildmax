package handlers

import (
	"context"
	"sync"
	"time"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	coreconv "github.com/gougoujiang/buildmax/internal/core/conversation"
	coreissue "github.com/gougoujiang/buildmax/internal/core/issue"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
	"github.com/gougoujiang/buildmax/internal/core/model"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/server/handlers/admin"
	artifactroutes "github.com/gougoujiang/buildmax/internal/server/handlers/artifact"
	authroutes "github.com/gougoujiang/buildmax/internal/server/handlers/auth"
	"github.com/gougoujiang/buildmax/internal/server/handlers/runterminal"
	teamroutes "github.com/gougoujiang/buildmax/internal/server/handlers/team"
	"github.com/gougoujiang/buildmax/internal/server/handlers/work"
	"github.com/gougoujiang/buildmax/internal/server/handlers/worker"
	"github.com/gougoujiang/buildmax/internal/server/turnqueue"
	wsconn "github.com/gougoujiang/buildmax/internal/server/websocket"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
	convchannel "github.com/gougoujiang/buildmax/internal/service/conversation/channel"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"
	"github.com/gougoujiang/buildmax/internal/service/quota"
)

// Config holds all dependencies for the unified handler (auth, user API, worker API, inbound webhook).
type Config struct {
	JWTSecret  string
	CORSOrigin string
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
	TeamStore         coreteam.Store
	WorkflowStore     coreworkflow.Store
	AgentStore        agentdef.Store
	IssueStore        coreissue.Store
	IssueCommentStore coreissue.CommentStore
	TaskStore         coretask.Store
	TaskRunStore      coretask.RunStore
	// LLMCallStore reads the managed call ledger. Nil leaves the ledger
	// unreadable over HTTP, which is what a deployment with no database has.
	LLMCallStore             coregw.CallStore
	RunOutputLister          work.RunOutputLister
	UserWebhookKeyStore      model.UserWebhookKeyStore
	ConversationStore        coreconv.Store
	ConversationMessageStore coreconv.MessageStore
	AuditStore               coreaudit.Store
	// LLMModelStore reads the managed model catalog. Nil leaves the admin
	// catalog routes answering 503. It is deliberately read plus enable/disable
	// here: the credential half of the catalog is edited by
	// `buildmax-server model`, on the machine that holds the database
	// credentials.
	LLMModelStore coregw.ModelStore

	// PluginService publishes Marketplace releases and manages catalog
	// entries. Nil leaves the catalog routes reporting that this deployment
	// has no Marketplace.
	PluginService *pluginsvc.Service
	// SchemaStore reports which migrations a database has had applied. Nil
	// leaves that field of the system status empty.
	SchemaStore model.SchemaStore
	// ArtifactStore records durable files. Nil, or no ArtifactStorage, leaves
	// the artifact routes answering 503: metadata without content is not an
	// artifact capability.
	ArtifactStore coreartifact.Store
	// SystemGrantStore reads deployment-scoped role grants. Nil leaves every
	// /api/admin route answering 503 to an authenticated caller, which is what
	// a deployment with no database has: no way to know whether anyone is an
	// administrator, and therefore no basis for letting one in.
	SystemGrantStore model.SystemGrantStore

	// Storage
	PersistStorage   blob.PersistStorage
	RunOutputStorage blob.RunOutputStorage
	ArtifactStorage  artifactsvc.ContentStore
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
	OnTaskRunTerminal func(ctx context.Context, info coretask.RunTerminalInfo)

	// TaskResultDeliveries records the reports the server owes finished runs.
	// Nil means a report that fails is not retried, which is what a deployment
	// with no database has.
	TaskResultDeliveries coretask.ResultDeliveryStore

	// Drain is closed when the server is going away. Watcher streams — the ones
	// that only observe state living in the database — end on it so they stop
	// holding the shutdown open, and so the browser knows to resubscribe
	// somewhere else. Streams that carry work in progress deliberately ignore
	// it. Nil means nothing ever drains, which is what a test has.
	// See docs/design/graceful-shutdown.md §5.
	Drain <-chan struct{}
}

// Handler serves all HTTP routes: auth, user API, worker API, inbound webhook.
type Handler struct {
	cfg          Config
	hub          wsconn.StreamHub
	connRegistry *wsconn.ConnRegistry
	// turns serializes the turns of one conversation and queues the rest. It is
	// server-scoped, not connection-scoped — see turnqueue.Registry.
	turns *turnqueue.Registry
	// terminal owns the callbacks a finished run fires. Server-scoped for the
	// same reason: a shutdown has to wait for all of them, not for the ones one
	// request happened to start.
	terminal *runterminal.Group

	// Surfaces and shared services are assembled once. A request executes a
	// capability; it never rebuilds the application's dependency graph.
	admin         *admin.Handler
	auth          *authroutes.Handler
	team          *teamroutes.Handler
	work          *work.Handler
	worker        *worker.Handler
	artifact      *artifactroutes.Handler
	artifacts     *artifactsvc.Service
	conversations *conversation.Service

	// sweeper retries the reports this server owes. Started by the server that
	// owns this handler, not by construction: a test builds handlers freely and
	// should not acquire a goroutine by doing so.
	sweepMu sync.Mutex
	sweeper *deliverySweeper
}

// NewHandler returns a configured Handler. If cfg.Hub is nil a new StreamHub is created internally.
func NewHandler(cfg Config) *Handler {
	hub := cfg.Hub
	if hub == nil {
		hub = wsconn.NewStreamHub()
	}
	h := &Handler{
		cfg:          cfg,
		hub:          hub,
		connRegistry: wsconn.NewConnRegistry(),
		turns:        turnqueue.NewRegistry(),
		terminal:     runterminal.NewGroup(),
	}
	h.artifacts = h.buildArtifactService()
	h.conversations = h.buildConversationService()
	h.admin = h.buildAdminHandler()
	h.auth = h.buildAuthHandler()
	h.team = h.buildTeamHandler()
	h.work = h.buildWorkHandler()
	h.worker = h.buildWorkerHandler()
	h.artifact = h.buildArtifactHandler()
	return h
}
