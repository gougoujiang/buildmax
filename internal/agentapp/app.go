package agentapp

import (
	"context"
	"errors"
	"fmt"
	"github.com/gougoujiang/buildmax/internal/core/subagent"
	"log/slog"
	"sync"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/infra/hook"
	llm "github.com/gougoujiang/buildmax/internal/infra/llm"
	"github.com/gougoujiang/buildmax/internal/infra/llmremote"
	"github.com/gougoujiang/buildmax/internal/infra/sandbox"
	"github.com/gougoujiang/buildmax/internal/infra/sessionstore"
	"github.com/gougoujiang/buildmax/internal/infra/trace"
	tools "github.com/gougoujiang/buildmax/internal/tool"
	"github.com/gougoujiang/buildmax/internal/util"
)

type AppConfig struct {
	WorkspaceDir string
	EnableMCP    bool
	// ModelEntries overrides settings.yaml models for this AgentApp. It is how a
	// surface in managed mode supplies what the deployment offers, and how a
	// worker receives the server's resolved model without writing credentials to
	// a run directory that is later persisted as an artifact.
	ModelEntries []config.ModelEntry
	// DefaultModel names the entry in ModelEntries a new session starts with.
	// Read only when ModelEntries is set; otherwise settings.yaml says.
	DefaultModel string
	// ManagedServerURL says these models are served by that deployment rather
	// than called from this machine. Empty means direct: the models are the ones
	// in settings.yaml and each carries its own provider credential.
	//
	// It is a property of the app rather than of an entry because a list has one
	// source. A surface is in one mode or the other, and the mode decides where
	// every prompt goes. See docs/design/client-modes.md section 4.
	ManagedServerURL string
	// Policy sets the tool permission policy for all runs in this AgentApp.
	// Nil defaults to AllowAllPolicy for backward compatibility.
	Policy agent.ToolPolicy
	// SandboxSurface picks the per-surface default sandbox baseline (see
	// config.SandboxSurfaceCLI / SandboxSurfaceWorker). Empty means
	// SandboxSurfaceCLI.
	SandboxSurface config.SandboxSurface
	// ManagedToken supplies the BuildMax credential for models configured with
	// transport "buildmax". Leaving it nil means this surface offers no managed
	// inference, and such an entry fails with a clear error instead of falling
	// back to a direct provider call.
	ManagedToken ManagedTokenFunc
	// ManagedTaskRunID makes managed calls from this app run-scoped: they go to
	// the worker route, carrying a run token instead of a login, and the server
	// derives user and team from it. Empty means managed calls are team-scoped,
	// which is what CLI, TUI, and Desktop do.
	ManagedTaskRunID string
	// Surface labels managed calls for correlation, e.g. "cli" or "desktop".
	Surface string
	// AdditionalSystemPrompt is free text appended to the system prompt as its last stable
	// layer: the user-authored identity and constraints for this run. It holds the prompt text
	// itself, not the name of anything. It is additive and never replaces the runtime prompt,
	// because replacing that would strip the tool-usage conventions the agent depends on and
	// the failure would look like a bad model rather than a bad configuration.
	//
	// Whoever assembles the run resolves it — a CLI flag, a named definition file, or the
	// agent record a task run names — and the last writer wins. It is bounded because it
	// lives in the system prompt, which is re-sent in full on every call and never trimmed.
	AdditionalSystemPrompt string
	// ArtifactPublisher gives this surface the artifact capability. Nil means it
	// has none — a session running straight against a model provider, with no
	// BuildMax server — and no artifact tool is registered at all.
	ArtifactPublisher tools.ArtifactPublisher
	// EnableBackgroundJobs turns on local background jobs: Bash gains
	// run_in_background and the Job tools are registered. Only interactive
	// surfaces (TUI, Desktop) set it — print mode has no host process to own
	// a job, and eval and workers have no unattended lifecycle for one, per
	// docs/design/local-background-jobs.md.
	EnableBackgroundJobs bool
}

// ManagedTokenFunc returns the BuildMax credential to use for serverURL. It is
// expected to refuse when the stored login belongs to a different server.
type ManagedTokenFunc func(serverURL string) (string, error)

type AgentApp struct {
	workspaceRoot          string
	settings               config.Settings
	llmClients             *LLMClientCache
	toolRegistriesMu       sync.Mutex
	toolRegistries         map[string]cllm.ToolRegistry
	mcpManager             *MCPManager
	skillsRegistry         *SkillRegistry
	subagentsRegistry      *SubAgentRegistry
	plugins                PluginSnapshot
	sessionManager         *SessionManager
	modelMu                sync.Mutex
	defaultModelOverride   string
	policy                 agent.ToolPolicy
	hooks                  agent.HookRunner
	sandbox                agent.SandboxView
	sandboxManager         *sandbox.Manager
	sandboxResolved        config.SandboxResolution
	additionalSystemPrompt string
	artifactPublisher      tools.ArtifactPublisher
	grantsMu               sync.Mutex
	grants                 map[string]*agent.SessionGrants
	turns                  turnCoordinator
	jobs                   *job.Manager
	// jobTraceDone closes once the job trace subscriber has drained the last
	// event. Close waits on it so no record is written after shutdown.
	jobTraceDone chan struct{}
	// openTraces holds the recorders of runs still in flight. A run closes its
	// own on the way out; this exists for the run that never gets there —
	// quitting the TUI mid-turn abandons the run goroutine, and its trace file
	// would otherwise stay open with no run_end.
	tracesMu   sync.Mutex
	openTraces map[*trace.Recorder]struct{}
}

// trackTrace registers a recorder as belonging to a run in flight.
func (a *AgentApp) trackTrace(r *trace.Recorder) {
	if a == nil || r == nil {
		return
	}
	a.tracesMu.Lock()
	defer a.tracesMu.Unlock()
	if a.openTraces == nil {
		a.openTraces = make(map[*trace.Recorder]struct{})
	}
	a.openTraces[r] = struct{}{}
}

// releaseTrace closes a recorder and forgets it. Safe to call twice: Recorder
// Close is idempotent, and the second call finds nothing registered.
func (a *AgentApp) releaseTrace(r *trace.Recorder) {
	if a == nil || r == nil {
		return
	}
	a.tracesMu.Lock()
	delete(a.openTraces, r)
	a.tracesMu.Unlock()
	r.Close()
}

// closeOpenTraces closes every trace whose run never finished.
func (a *AgentApp) closeOpenTraces() {
	if a == nil {
		return
	}
	a.tracesMu.Lock()
	pending := make([]*trace.Recorder, 0, len(a.openTraces))
	for r := range a.openTraces {
		pending = append(pending, r)
	}
	a.openTraces = nil
	a.tracesMu.Unlock()
	for _, r := range pending {
		r.RecordRunEnd("run abandoned: application shut down before the turn finished")
		r.Close()
	}
}

// Jobs returns the app's background job manager, or nil where background
// jobs are disabled. One manager per AgentApp: jobs are process-scoped but
// owned by this workspace's runtime, and closing the app stops them.
func (a *AgentApp) Jobs() *job.Manager {
	return a.jobs
}

// grantsFor returns the approval grants for one session, creating the store on
// first use. Keyed by session rather than held on SessionContext because
// Desktop rebuilds that wrapper on every message, and a grant that does not
// outlive the turn it was given in is not a session grant.
//
// Entries are never evicted. One store is a small map, and the alternative is a
// session-close hook that no surface currently has.
func (a *AgentApp) grantsFor(sessionID string) *agent.SessionGrants {
	if a == nil || sessionID == "" {
		return nil
	}
	a.grantsMu.Lock()
	defer a.grantsMu.Unlock()
	if a.grants == nil {
		a.grants = make(map[string]*agent.SessionGrants)
	}
	g := a.grants[sessionID]
	if g == nil {
		g = agent.NewSessionGrants()
		a.grants[sessionID] = g
	}
	return g
}

type SkillRegistry struct {
	entries  []tools.SkillEntry
	shadowed []plugin.Shadowed
	findings []plugin.Finding
}

type SubAgentRegistry struct {
	userDefs []subagent.Def
	shadowed []plugin.Shadowed
	findings []plugin.Finding
}

type LLMClientCache struct {
	settings config.Settings
	// managedServerURL is set when this app's models are served by a deployment.
	// Empty means every model here is called directly with its own credential.
	managedServerURL string
	// managedToken supplies the BuildMax credential. Nil means this surface
	// cannot authenticate to a deployment — the evaluation harness deliberately
	// does not.
	managedToken ManagedTokenFunc
	// managedTaskRunID scopes managed calls to one task run, which sends them to
	// the worker route. Empty means they carry the user's own login.
	managedTaskRunID string
	// surface labels managed calls for correlation only.
	surface string
	mu      sync.Mutex
	clients map[string]cllm.LLMClient
}

type RunResult struct {
	Reply                 string
	Duration              time.Duration
	ToolCalls             int
	PromptTokens          int
	CompletionTokens      int
	TotalPromptTokens     int
	TotalCompletionTokens int
	// Cache counts are the provider-reported cached parts of the prompt totals
	// beside them, not extra tokens. Zero means the provider reported none,
	// which is not the same fact as a cache miss.
	CacheReadTokens       int
	CacheWriteTokens      int
	TotalCacheReadTokens  int
	TotalCacheWriteTokens int
	// Cost is the session's estimated spend so far, nil when nothing in it
	// could be priced. It is the session total rather than this turn's,
	// because that is what the session file accumulates: a per-turn figure
	// would need rates that may have changed since the turn ran.
	Cost *cllm.Cost
	// CostIncomplete says part of the session could not be priced, so the
	// total above understates it.
	CostIncomplete bool
	ContextTokens  int
	ContextWindow  int
	SessionID      string
	Workspace      string
	ModelName      string
	// TraceID identifies the durable run trace written for this run, or "" when
	// tracing is disabled or failed to start. Points at
	// <DataDir>/traces/<session_id>/<trace_id>.jsonl.
	TraceID string
	// TracePath is that file's path on disk, or "" when no trace was written.
	// Callers that persist a reference to the trace use this instead of
	// rebuilding the layout from TraceID, so the stored path and the written
	// file cannot disagree.
	TracePath string
}

type RunStatus struct {
	ContextTokens         int `json:"context_tokens"`
	ContextWindow         int `json:"context_window"`
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalPromptTokens     int `json:"total_prompt_tokens"`
	TotalCompletionTokens int `json:"total_completion_tokens"`
	// Cache counts break the prompt counts beside them down; summing them with
	// the prompt total counts the same tokens twice.
	CacheReadTokens       int `json:"cache_read_tokens"`
	CacheWriteTokens      int `json:"cache_write_tokens"`
	TotalCacheReadTokens  int `json:"total_cache_read_tokens"`
	TotalCacheWriteTokens int `json:"total_cache_write_tokens"`
	// Cost is the session's estimated spend, absent when nothing could be
	// priced. CostIncomplete says the figure is missing part of the session.
	Cost           *cllm.Cost `json:"cost,omitempty"`
	CostIncomplete bool       `json:"cost_incomplete,omitempty"`
}

type TurnFinalizeResult struct {
	Title            string
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	CacheWriteTokens int
}

// ModelConfig is one resolved model entry usable for client creation.
type ModelConfig struct {
	Name          string
	ProviderModel string
	BaseURL       string
	APIKey        string
	ContextWindow int // 0 = no windowing; from settings.yaml model entry
	CallTimeout   int // seconds; 0 = uses DefaultCallTimeoutSecs
	MaxTokens     int // 0 = the adapter's own default
	// Reasoning is the effort level (config.Reasoning*); off means none.
	Reasoning string
	// CacheControl is the resolved prompt-cache policy: which calls ask the
	// provider to cache the stable prefix, and for how long. Resolved here
	// rather than in the client so one place folds the deprecated
	// prompt_cache shorthand.
	CacheControl config.CacheControl
	// Pricing is what this model charges. Zero means the entry configured no
	// prices, and a run against it reports its cost as unavailable rather than
	// as zero — BuildMax does not know what any provider charges.
	Pricing cllm.Pricing
	// PricingErr is why an entry's prices could not be read, empty when they
	// could. Carried rather than returned because a malformed price must not
	// stop a model from answering: the run still works, it just cannot be
	// costed, and the surface says so instead of failing the turn.
	PricingErr string
	// Integration names a qualified OpenAI-compatible gateway; empty is the
	// normal case.
	Integration string
	// Vision says this model accepts image input.
	Vision bool
	// KeepAlive is how long a local runtime keeps the model loaded between
	// calls. Only a local provider has one to keep.
	KeepAlive string
	// Provider is the wire protocol this model speaks. Empty means
	// config.LLMProviderOpenAICompatible. In managed mode it is ignored: the
	// operator's catalog decides which protocol serves the call.
	//
	// There is no transport here. Where a prompt goes is a property of the app's
	// mode, not of one model — see AppConfig.ManagedServerURL.
	Provider string
}

// ManagedServerURL is the deployment serving this app's models, or empty when
// they are called directly from this machine. It is the app's mode, and a
// surface names it wherever it tells the user where a prompt goes.
func (a *AgentApp) ManagedServerURL() string {
	if a == nil || a.llmClients == nil {
		return ""
	}
	return a.llmClients.managedServerURL
}

// effectiveAdditionalPrompt returns the additional system prompt a run uses: the one this app
// was configured with, or — when it was given none — the one the session already ran under. A
// resumed session keeps its identity rather than losing it because the flag that set it was not
// repeated. A configured value wins, which is what makes an edited Portal agent definition take
// effect on the next run.
func (a *AgentApp) effectiveAdditionalPrompt(sess *SessionContext) string {
	if a == nil {
		return ""
	}
	if a.additionalSystemPrompt != "" {
		return a.additionalSystemPrompt
	}
	if sess != nil {
		return sess.AdditionalPrompt()
	}
	return ""
}

func NewAgentApp(cfg AppConfig) (*AgentApp, error) {
	workspaceRoot, err := resolveWorkspaceRoot(cfg.WorkspaceDir)
	if err != nil {
		return nil, err
	}
	if err := ValidateAdditionalSystemPrompt(cfg.AdditionalSystemPrompt); err != nil {
		return nil, err
	}
	settings, err := config.LoadSettings()
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	if len(cfg.ModelEntries) > 0 {
		// A supplied list replaces settings.yaml's outright, default included:
		// in managed mode the deployment says which of its models is the
		// default, and a name from the local file would select nothing here.
		settings.Models = append([]config.ModelEntry(nil), cfg.ModelEntries...)
		settings.DefaultModel = cfg.DefaultModel
	}
	// Resolved once, here: every layer below reads the same inventory, and a
	// plugin installed later must not change a runtime already assembled.
	pluginSnapshot := discoverPlugins()
	pluginSnapshot.resolveBase(context.Background())
	loadedPlugins := pluginSnapshot.Loadable()

	wsHooks, err := config.LoadWorkspaceHooks(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("load workspace hooks: %w", err)
	}
	pluginHooks := config.ResolvePluginHooks(loadedPlugins)
	pluginSnapshot.addFindings(pluginHooks.Findings...)
	mergedHooks := config.MergeHooks(settings.Hooks, pluginHooks.Config, wsHooks)

	policyCfg, err := config.LoadPolicySandbox()
	if err != nil {
		return nil, fmt.Errorf("load policy sandbox: %w", err)
	}
	surface := cfg.SandboxSurface
	if surface == "" {
		surface = config.SandboxSurfaceCLI
	}
	sandboxResolved := config.ResolveSandbox(settings.Sandbox, policyCfg, surface)
	sandboxManager, err := buildSandboxManager(sandboxResolved, workspaceRoot)
	if err != nil {
		return nil, err
	}
	var sandboxView agent.SandboxView = sandboxManager

	app := &AgentApp{
		workspaceRoot:          workspaceRoot,
		settings:               settings,
		toolRegistries:         make(map[string]cllm.ToolRegistry),
		sessionManager:         NewSessionManager(config.SessionsDir()),
		skillsRegistry:         &SkillRegistry{},
		subagentsRegistry:      &SubAgentRegistry{},
		policy:                 NewConfiguredPolicy(config.ResolvePermissions(settings.Tools), cfg.Policy),
		additionalSystemPrompt: cfg.AdditionalSystemPrompt,
		artifactPublisher:      cfg.ArtifactPublisher,
		sandbox:                sandboxView,
		sandboxManager:         sandboxManager,
		sandboxResolved:        sandboxResolved,
		plugins:                pluginSnapshot,
	}
	app.llmClients = &LLMClientCache{
		settings:         app.settings,
		managedServerURL: cfg.ManagedServerURL,
		managedToken:     cfg.ManagedToken,
		managedTaskRunID: cfg.ManagedTaskRunID,
		surface:          cfg.Surface,
		clients:          make(map[string]cllm.LLMClient),
	}
	if cfg.EnableMCP {
		mcpRes, err := config.ResolveMCPConfig(app.workspaceRoot, loadedPlugins)
		if err != nil {
			return nil, fmt.Errorf("load mcp config: %w", err)
		}
		app.plugins.addFindings(mcpRes.Findings...)
		app.plugins.addShadowed(mcpRes.Shadowed...)
		app.mcpManager, err = NewMCPManager(context.Background(), mcpRes.Config)
		if err != nil {
			return nil, err
		}
	}
	// HookManager wiring happens after MCP + LLMClientCache exist so the
	// mcp_tool and prompt drivers can be backed by adapters in this package.
	hookDeps := hook.Deps{LLMCaller: &llmCaller{cache: app.llmClients, defaultModel: app.DefaultModelName}}
	if app.mcpManager != nil {
		hookDeps.MCPCaller = &mcpCaller{m: app.mcpManager}
	}
	app.hooks = NewHookManager(mergedHooks, hook.NewDriverRegistry(hookDeps))

	if err := app.skillsRegistry.Load(app.workspaceRoot, loadedPlugins); err != nil {
		return nil, err
	}
	if err := app.subagentsRegistry.Load(app.workspaceRoot, loadedPlugins); err != nil {
		return nil, err
	}
	app.plugins.addFindings(app.skillsRegistry.findings...)
	app.plugins.addShadowed(app.skillsRegistry.shadowed...)
	app.plugins.addFindings(app.subagentsRegistry.findings...)
	app.plugins.addShadowed(app.subagentsRegistry.shadowed...)
	if cfg.EnableBackgroundJobs {
		app.jobs = job.NewManager()
		if config.TraceEnabled() {
			events, _ := app.jobs.Subscribe("")
			app.jobTraceDone = make(chan struct{})
			go func() {
				defer close(app.jobTraceDone)
				logJobEvents(config.TracesDir(), events)
			}()
		}
	}
	return app, nil
}

// jobShutdownTimeout bounds how long Close waits for background jobs after
// their own TERM-to-KILL escalation. Generous enough for the kill to land,
// short enough that quitting the app never hangs.
const jobShutdownTimeout = 10 * time.Second

func (a *AgentApp) Close() error {
	if a == nil {
		return nil
	}
	var firstErr error
	if a.jobs != nil {
		ctx, cancel := context.WithTimeout(context.Background(), jobShutdownTimeout)
		if err := a.jobs.Close(ctx); err != nil {
			firstErr = err
		}
		cancel()
		// The manager releases the subscription, but the trace writer drains
		// what is still buffered. Returning before that lands leaves the last
		// job_end record racing whatever runs next against the traces dir.
		if a.jobTraceDone != nil {
			<-a.jobTraceDone
		}
	}
	if a.mcpManager != nil {
		if err := a.mcpManager.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if a.sandboxManager != nil {
		if err := a.sandboxManager.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// Last, so a run still unwinding has had every chance to close its own
	// trace first and record a real run_end rather than the abandoned one.
	a.closeOpenTraces()
	return firstErr
}

// Sandbox returns the SandboxView the agent will run with. In Phase A this
// is always NoopSandbox; Phase B will install the OS-backed manager.
func (a *AgentApp) Sandbox() agent.SandboxView {
	if a == nil || a.sandbox == nil {
		return agent.NoopSandbox{}
	}
	return a.sandbox
}

// SandboxResolution returns the resolved config plus the per-layer source
// chain. Surfaced by `buildmax sandbox status`.
func (a *AgentApp) SandboxResolution() config.SandboxResolution {
	if a == nil {
		return config.SandboxResolution{}
	}
	return a.sandboxResolved
}

func (a *AgentApp) WorkspaceRoot() string {
	if a == nil {
		return ""
	}
	return a.workspaceRoot
}

func (a *AgentApp) SessionsDir() string {
	if a == nil || a.sessionManager == nil {
		return ""
	}
	return a.sessionManager.dir
}

func (a *AgentApp) SkillEntries() []tools.SkillEntry {
	if a == nil || a.skillsRegistry == nil {
		return nil
	}
	return a.skillsRegistry.Entries()
}

func (a *AgentApp) DefaultModelName() string {
	if a == nil {
		return ""
	}
	a.modelMu.Lock()
	override := a.defaultModelOverride
	a.modelMu.Unlock()
	if override != "" {
		return override
	}
	return DefaultModelName(a.settings)
}

// SetDefaultModel overrides the model used for new turns in this AgentApp.
func (a *AgentApp) SetDefaultModel(name string) {
	if a == nil {
		return
	}
	a.modelMu.Lock()
	a.defaultModelOverride = name
	a.modelMu.Unlock()
}

// AgentDefs returns the user-defined sub-agent definitions for this workspace.
func (a *AgentApp) AgentDefs() []subagent.Def {
	if a == nil || a.subagentsRegistry == nil {
		return nil
	}
	return a.subagentsRegistry.Definitions()
}

func (a *AgentApp) ModelConfigs() []ModelConfig {
	if a == nil {
		return nil
	}
	if len(a.settings.Models) == 0 {
		if cfg, ok := DefaultModelConfig(a.settings); ok {
			return []ModelConfig{cfg}
		}
		return nil
	}
	out := make([]ModelConfig, 0, len(a.settings.Models))
	for _, entry := range a.settings.Models {
		out = append(out, toModelConfig(entry))
	}
	return out
}

// ToolEntry is a name+description pair for a tool available to the agent.
type ToolEntry struct {
	Name        string
	Description string
	// Access is what the tool says the call does: "read-only" or "write".
	Access string
	// Action is what the call resolves to with no arguments and a human
	// present: "allow", "ask", or "deny". Argument-dependent tools can resolve
	// differently for a real call — Bash asks only for a risky command — so
	// this is the category answer, not a promise about every invocation.
	Action string
	// Source names where Action came from: "settings" or "derived".
	Source string
}

// ToolEntries returns the name and description of every tool available to the agent.
// It reuses the cached tool registry when available; otherwise it builds one.
func (a *AgentApp) ToolEntries() []ToolEntry {
	if a == nil {
		return nil
	}
	a.toolRegistriesMu.Lock()
	var registry cllm.ToolRegistry
	for _, r := range a.toolRegistries {
		registry = r
		break
	}
	a.toolRegistriesMu.Unlock()

	if len(registry.Tools()) == 0 {
		client, _ := a.llmClients.Get(a.DefaultModelName())
		registry, _ = a.buildToolRegistry(client)
	}

	all := registry.Tools()
	entries := make([]ToolEntry, 0, len(all))
	for _, t := range all {
		entries = append(entries, ToolEntry{
			Name:        t.Name(),
			Description: t.Description(),
			Access:      agent.DeclaredAccess(t, nil).String(),
			Action:      actionLabel(agent.ResolveToolAction(a.policy, t, nil, true)),
			Source:      a.permissionSource(t.Name()),
		})
	}
	return entries
}

func actionLabel(a cllm.ToolAction) string {
	switch a {
	case cllm.ToolActionDeny:
		return config.PermissionDeny
	case cllm.ToolActionAsk:
		return config.PermissionAsk
	default:
		return config.PermissionAllow
	}
}

// permissionSource names the layer that decided a tool's category action.
func (a *AgentApp) permissionSource(name string) string {
	if e, ok := config.ResolvePermissions(a.settings.Tools).Lookup(name, ""); ok {
		return e.Source
	}
	return "derived"
}

// PermissionRules returns the configured rules in resolution order, for display
// alongside ToolEntries. Rules naming a dispatch target have no tool row of
// their own.
func (a *AgentApp) PermissionRules() []config.PermissionEntry {
	if a == nil {
		return nil
	}
	return config.ResolvePermissions(a.settings.Tools).Entries
}

// PermissionIssues returns rules that were ignored because their action was not
// recognised. A rule silently dropped looks exactly like one that is in force.
func (a *AgentApp) PermissionIssues() []string {
	if a == nil {
		return nil
	}
	return config.ResolvePermissions(a.settings.Tools).Invalid
}

func (a *AgentApp) MCPStatus() MCPStatus {
	if a == nil || a.mcpManager == nil {
		return MCPStatus{}
	}
	return a.mcpManager.Status()
}

func (a *AgentApp) RefreshMCP(ctx context.Context) (MCPStatus, error) {
	if a == nil || a.mcpManager == nil {
		return MCPStatus{}, nil
	}
	// Refresh re-reads the files, not the plugin inventory: a runtime keeps the
	// snapshot it was assembled with.
	mcpRes, err := config.ResolveMCPConfig(a.workspaceRoot, a.plugins.Loadable())
	if err != nil {
		return MCPStatus{}, fmt.Errorf("load mcp config: %w", err)
	}
	if err := a.mcpManager.Refresh(ctx, mcpRes.Config); err != nil {
		return MCPStatus{}, err
	}
	a.toolRegistriesMu.Lock()
	a.toolRegistries = make(map[string]cllm.ToolRegistry)
	a.toolRegistriesMu.Unlock()
	return a.mcpManager.Status(), nil
}

func (a *AgentApp) OpenSession(sessionID string) (*SessionContext, error) {
	if a == nil || a.sessionManager == nil {
		return nil, fmt.Errorf("session store is not initialized")
	}
	var (
		sess *SessionContext
		err  error
	)
	if sessionID == "" {
		sess, err = a.sessionManager.Create(a.DefaultModelName())
	} else {
		sess, err = a.sessionManager.Open(sessionID, a.DefaultModelName())
	}
	if err != nil {
		return nil, err
	}
	a.fireSessionLifecycle(agent.HookSessionStart, sess, nil)
	return sess, nil
}

// OpenOrCreateSession loads sessionID when it has been persisted, or creates a
// new session with that ID. Remote task runs use this because the server assigns
// a session ID before the worker has written the first session file.
func (a *AgentApp) OpenOrCreateSession(sessionID string) (*SessionContext, error) {
	if sessionID == "" {
		return a.OpenSession("")
	}
	sess, err := a.OpenSession(sessionID)
	if err == nil {
		return sess, nil
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		return nil, err
	}
	sess, err = a.sessionManager.CreateWithID(sessionID, a.DefaultModelName())
	if err != nil {
		return nil, err
	}
	a.fireSessionLifecycle(agent.HookSessionStart, sess, nil)
	return sess, nil
}

// CloseSession releases a finished session and fires the SessionEnd hook.
//
// Every OpenSession must be paired with one of these. An open session holds the
// writer lock and its journal file, so leaving one open keeps the session
// unopenable by anything else — including this process, which is how a second
// open of the same id fails rather than waits. Safe to call with a nil session,
// and safe to call twice.
func (a *AgentApp) CloseSession(sess *SessionContext) {
	if a == nil || sess == nil {
		return
	}
	a.fireSessionLifecycle(agent.HookSessionEnd, sess, nil)
	// The hook runs first so it still sees a session it can read; closing only
	// releases the lock and the file, not the in-memory state.
	if err := sess.Close(); err != nil {
		slog.Warn("closing the session failed", "session_id", sess.ID(), "err", err)
	}
}

// ReadSession loads a session without taking its writer lock, for callers that
// only display it.
//
// A status view must work while a turn is running, so it cannot be the thing
// that takes the lock. The result cannot be written to: it is a read model, and
// its commit paths are no-ops, which is why it is a different call rather than
// a flag on OpenSession.
func (a *AgentApp) ReadSession(sessionID string) (*SessionContext, error) {
	if a == nil || a.sessionManager == nil {
		return nil, fmt.Errorf("session store is not initialized")
	}
	loaded, err := a.sessionManager.Load(sessionID, session.LoadFull)
	if err != nil {
		return nil, err
	}
	return newReadOnlyContext(loaded, a.DefaultModelName()), nil
}

func (a *AgentApp) fireSessionLifecycle(event agent.HookEvent, sess *SessionContext, stats *agent.RunStats) {
	if a == nil || a.hooks == nil || sess == nil {
		return
	}
	in := agent.HookInput{
		Event:     event,
		SessionID: sess.ID(),
		Workspace: a.workspaceRoot,
		Sandbox:   a.sandboxInfo(),
	}
	if stats != nil {
		in.Stats = stats
	}
	a.hooks.Run(context.Background(), in)
}

// sandboxInfo snapshots the active SandboxView into the hook payload
// shape. Returns nil when no SandboxView is wired (older test paths).
func (a *AgentApp) sandboxInfo() *agent.SandboxInfo {
	if a == nil {
		return nil
	}
	view := a.Sandbox()
	if view == nil {
		return nil
	}
	return &agent.SandboxInfo{
		Enabled: view.Enabled(),
		Mode:    view.Mode(),
		Backend: view.Backend(),
		Sources: append([]string(nil), a.sandboxResolved.Sources...),
	}
}

func (a *AgentApp) EstimateRunStatus(sess *SessionContext) (RunStatus, error) {
	if a == nil {
		return RunStatus{}, fmt.Errorf("app is nil")
	}
	if sess == nil {
		sess = NewSessionContext(a.DefaultModelName())
	}
	modelName := sess.ModelName(a.DefaultModelName())
	client, err := a.llmClients.Get(modelName)
	if err != nil {
		return RunStatus{}, err
	}
	return a.estimateRunStatus(sess, modelName, client.ContextWindow()), nil
}

func (a *AgentApp) estimateRunStatus(sess *SessionContext, modelName string, contextWindow int) RunStatus {
	if a == nil || sess == nil {
		return RunStatus{}
	}
	// This path does not go through RunLoop, so it renders the compaction block itself to
	// estimate the real prompt size. It uses the same renderer RunLoop does.
	systemPrompt := BuildEffectiveSystemPrompt(a.workspaceRoot, modelName, a.effectiveAdditionalPrompt(sess), a.promptCapabilities()) + agent.RenderCompactionBlock(sess.PriorSummary())
	contextTokens := agent.EstimateMessageTokens(cllm.Message{Role: "system", Content: systemPrompt}) + agent.EstimateTokens(sess.HistoryMessages())
	return RunStatus{
		ContextTokens:         contextTokens,
		ContextWindow:         contextWindow,
		TotalPromptTokens:     sess.PromptTokens(),
		TotalCompletionTokens: sess.CompletionTokens(),
		TotalCacheReadTokens:  sess.CacheReadTokens(),
		TotalCacheWriteTokens: sess.CacheWriteTokens(),
		Cost:                  sess.Cost(),
		CostIncomplete:        sess.CostIncomplete(),
	}
}

func (a *AgentApp) resolveRunContext(sess *SessionContext) (*SessionContext, string, cllm.LLMClient, error) {
	if a == nil {
		return nil, "", nil, fmt.Errorf("app is nil")
	}
	if sess == nil {
		sess = NewSessionContext(a.DefaultModelName())
	}
	modelName := sess.ModelName(a.DefaultModelName())
	client, err := a.llmClients.Get(modelName)
	if err != nil {
		return nil, "", nil, err
	}
	return sess, modelName, client, nil
}

// RunPromptOpts is the optional per-run wiring a surface supplies. The zero value
// is a valid non-interactive run: no streaming, no approvals, no events.
type RunPromptOpts struct {
	// Stream receives content deltas. Nil runs the LLM in blocking mode.
	Stream cllm.StreamSink
	// Approval resolves tool calls the policy sends to "ask". Nil collapses ask
	// to deny, which is what a surface with nobody to ask should do.
	Approval agent.ApprovalHandler
	// EventSink receives runtime events. Nil disables the caller's leg only — the
	// durable trace records the run either way.
	EventSink func(agent.Event)
	// Pending carries messages the user submits while this run is working. They
	// are appended to the history at the next iteration boundary instead of
	// waiting for the run to finish. Nil disables mid-run injection, leaving the
	// surface to drain its own queue between runs.
	Pending agent.PendingInput
}

// traceRunContext is the trace identity a running tool inherits when it starts
// a subagent. It is context-local rather than held on AgentApp because one app
// can run several sessions concurrently, each with its own parent run.
type traceRunContext struct {
	runID     string
	modelName string
}

type traceRunContextKey struct{}

func withTraceRunContext(ctx context.Context, runID, modelName string) context.Context {
	if runID == "" {
		return ctx
	}
	// The core-level run ID travels alongside: tools that detach owned work
	// (background jobs) read provenance through core/agent, which cannot see
	// this package's context key.
	ctx = agent.CtxWithRunID(ctx, runID)
	return context.WithValue(ctx, traceRunContextKey{}, traceRunContext{runID: runID, modelName: modelName})
}

func traceRunFromContext(ctx context.Context) traceRunContext {
	traceRun, _ := ctx.Value(traceRunContextKey{}).(traceRunContext)
	return traceRun
}

func (a *AgentApp) RunPrompt(ctx context.Context, sess *SessionContext, prompt string, opts RunPromptOpts) (RunResult, error) {
	return a.runTurn(ctx, sess, prompt, nil, opts)
}

// RunBackgroundEvent runs one serialized turn caused by a background job
// event rather than a user prompt. The appended message carries the event's
// non-user Source and an envelope framing the payload as untrusted
// observation; UserPromptSubmit does not fire, because nothing here is a
// user prompt. Serialization against the session is the same as RunPrompt's.
func (a *AgentApp) RunBackgroundEvent(ctx context.Context, sess *SessionContext, ev BackgroundEvent, opts RunPromptOpts) (RunResult, error) {
	return a.runTurn(ctx, sess, "", &ev, opts)
}

func (a *AgentApp) runTurn(ctx context.Context, sess *SessionContext, prompt string, event *BackgroundEvent, opts RunPromptOpts) (RunResult, error) {
	sess, modelName, client, err := a.resolveRunContext(sess)
	if err != nil {
		return RunResult{}, err
	}
	// One writer per session. Surfaces queue prompts behind the active run,
	// so a concurrent call here is a caller bug or an unserialized background
	// producer — refused, because Session and SessionManager have no locks of
	// their own and an overlapping turn would race the history.
	if err := a.turns.begin(sess.ID()); err != nil {
		return RunResult{SessionID: sess.ID()}, fmt.Errorf("session %s: %w", sess.ID(), err)
	}
	defer a.turns.end(sess.ID())
	registry, err := a.toolRegistry(modelName, client)
	if err != nil {
		return RunResult{}, err
	}
	start := time.Now()
	ctx = session.CtxWithSessionID(ctx, sess.ID())
	// NoteWrite and TodoWrite reach the session through the context: the tool registry is
	// cached per model and shared across sessions, so a tool must not hold one.
	ctx = agent.CtxWithNoteStore(ctx, sess)

	// Resolved before the trace opens, because the trace reports which prompt layers this run
	// loaded and a run that ends early still has to be able to say.
	extraPrompt := a.effectiveAdditionalPrompt(sess)
	systemPrompt, promptLayers := BuildSystemPromptWithLayers(a.workspaceRoot, modelName, extraPrompt, a.promptCapabilities())
	// Durable state, so it commits rather than being assigned: a resumed
	// session that lost the prompt it ran under would answer as a different
	// agent than the one the conversation records.
	if err := sess.SetAdditionalPrompt(extraPrompt); err != nil {
		return RunResult{SessionID: sess.ID()}, fmt.Errorf("persist session: %w", err)
	}

	// Durable run trace: one JSONL file per run, attached at this single
	// chokepoint so CLI/TUI, Desktop, eval, and the worker all produce traces
	// with no per-surface code. Fail-open — a nil recorder is a no-op.
	var recorder *trace.Recorder
	if config.TraceEnabled() {
		// Tracing is fail-open, so entropy failure costs the trace and not the
		// run: an empty run ID makes NewRecorder disable itself and say so.
		runID, _ := util.NewPublicID()
		// Traces live inside the session's own bundle, one file per run, so a
		// session's diagnostics are deleted, copied, and retained with the
		// conversation they describe rather than from a second root.
		recorder = trace.NewRecorder(sessionstore.SessionTracesDir(a.sessionManager.Dir(), sess.ID()), trace.Meta{
			RunID:        runID,
			SessionID:    sess.ID(),
			Workspace:    a.workspaceRoot,
			Model:        modelName,
			Sandbox:      a.sandboxInfo(),
			PromptLayers: promptLayers,
			Plugins:      a.plugins.Provenance(ctx),
		})
		a.trackTrace(recorder)
		defer a.releaseTrace(recorder)
		ctx = withTraceRunContext(ctx, recorder.RunID(), modelName)
	}

	// Give hooks a chance to inspect / reject the prompt before it enters
	// history or the LLM. A block short-circuits the turn: the prompt is
	// not appended, the LLM is not called, and the user receives the
	// hook's reason as the reply. Background events skip this: they are not
	// user prompts, and running a user-prompt hook on them would apply the
	// wrong contract.
	if event == nil {
		promptHook := a.hooks.Run(ctx, agent.HookInput{
			Event:     agent.HookUserPromptSubmit,
			SessionID: sess.ID(),
			Workspace: a.workspaceRoot,
			Prompt:    prompt,
		})
		if promptHook.Blocked() {
			reason := promptHook.Reason
			if reason == "" {
				reason = "prompt blocked by hook"
			}
			recorder.RecordRunEnd("blocked by hook: " + reason)
			status := a.estimateRunStatus(sess, modelName, client.ContextWindow())
			return RunResult{
				Reply:                 reason,
				Duration:              time.Since(start),
				TotalPromptTokens:     sess.PromptTokens(),
				TotalCompletionTokens: sess.CompletionTokens(),
				TotalCacheReadTokens:  sess.CacheReadTokens(),
				TotalCacheWriteTokens: sess.CacheWriteTokens(),
				Cost:                  sess.Cost(),
				CostIncomplete:        sess.CostIncomplete(),
				ContextTokens:         status.ContextTokens,
				ContextWindow:         status.ContextWindow,
				SessionID:             sess.ID(),
				Workspace:             a.workspaceRoot,
				ModelName:             modelName,
				TraceID:               recorder.RunID(),
				TracePath:             recorder.Path(),
			}, nil
		}
	}

	turnMsg := cllm.Message{Role: "user", Content: prompt}
	if event != nil {
		turnMsg = event.message()
	}
	if err := sess.Append(turnMsg); err != nil {
		return RunResult{}, err
	}

	// No compaction block here: RunLoop reads the stored summary through
	// CompactionHistory and renders it itself. Appending it here as well put two
	// <context_compaction> blocks in the prompt after the first in-run compaction.
	reply, stats, err := agent.RunLoop(ctx, agent.RunLoopOpts{
		LLMClient:    client,
		Pricing:      a.pricingFor(sess),
		SystemPrompt: systemPrompt,
		ToolRegistry: registry,
		MaxIter:      agent.DefaultMaxIterations,
		History:      sess,
		StreamSink:   opts.Stream,
		Policy:       a.policy,
		Approval:     opts.Approval,
		PendingInput: opts.Pending,
		Grants:       a.grantsFor(sess.ID()),

		MaxParallelTools: config.ResolveMaxParallelTools(a.settings.Agent),
		Compactor:        NewLLMCompactor(client),
		Checkpointer:     NewNoteCheckpointer(client),
		Invariants:       agent.ExtractInvariants(extraPrompt),
		EventSink:        teeEventSink(recorder.Record, opts.EventSink),
		Hooks:            a.hooks,
		SessionID:        sess.ID(),
		Workspace:        a.workspaceRoot,
	})
	// Failed runs still leave a complete trace (RunLoop emits run_end with the
	// error), so carry TraceID out even on the error paths — a failed run is
	// exactly when the caller most needs to point a viewer at the file.
	if err != nil {
		return RunResult{SessionID: sess.ID(), TraceID: recorder.RunID(), TracePath: recorder.Path()}, fmt.Errorf("agent: %w", err)
	}
	if _, err := a.finalizeTurn(sess, client, stats); err != nil {
		return RunResult{SessionID: sess.ID(), TraceID: recorder.RunID(), TracePath: recorder.Path()}, err
	}
	status := a.estimateRunStatus(sess, modelName, client.ContextWindow())
	return RunResult{
		Reply:                 reply,
		Duration:              time.Since(start),
		ToolCalls:             stats.ToolCalls,
		PromptTokens:          stats.PromptTokens,
		CompletionTokens:      stats.CompletionTokens,
		CacheReadTokens:       stats.CacheReadTokens,
		CacheWriteTokens:      stats.CacheWriteTokens,
		TotalPromptTokens:     sess.PromptTokens(),
		TotalCompletionTokens: sess.CompletionTokens(),
		TotalCacheReadTokens:  sess.CacheReadTokens(),
		TotalCacheWriteTokens: sess.CacheWriteTokens(),
		Cost:                  sess.Cost(),
		CostIncomplete:        sess.CostIncomplete(),
		ContextTokens:         status.ContextTokens,
		ContextWindow:         status.ContextWindow,
		SessionID:             sess.ID(),
		Workspace:             a.workspaceRoot,
		ModelName:             modelName,
		TraceID:               recorder.RunID(),
		TracePath:             recorder.Path(),
	}, nil
}

// teeEventSink fans one runtime event out to the trace recorder first (so a
// panic in the caller's sink cannot lose trace data) and then to the caller's
// sink. Either leg may be nil.
func teeEventSink(record, caller func(agent.Event)) func(agent.Event) {
	if record == nil {
		return caller
	}
	return func(e agent.Event) {
		record(e)
		if caller != nil {
			caller(e)
		}
	}
}

func (a *AgentApp) GenerateSessionTitle(ctx context.Context, sess *SessionContext) (string, cllm.Usage, error) {
	if a == nil || sess == nil {
		return "", cllm.Usage{}, nil
	}
	_, _, client, err := a.resolveRunContext(sess)
	if err != nil {
		return "", cllm.Usage{}, err
	}
	return a.sessionManager.GenerateTitle(ctx, client, sess)
}

func (a *AgentApp) finalizeTurn(sess *SessionContext, client cllm.LLMClient, stats agent.RunStats) (TurnFinalizeResult, error) {
	return a.sessionManager.Finalize(context.Background(), client, sess, a.workspaceRoot, stats, a.pricingFor(sess))
}

// pricingFor is the price list of the model this session is running against, or
// the zero Pricing when the entry configured none. A managed entry has none
// here on purpose: the server holds the rates for a managed call and records
// what it charged on the ledger, so a local guess would be a second answer to a
// question that already has one.
func (a *AgentApp) pricingFor(sess *SessionContext) cllm.Pricing {
	if a == nil || sess == nil {
		return cllm.Pricing{}
	}
	if a.ManagedServerURL() != "" {
		return cllm.Pricing{}
	}
	cfg, ok := FindModelConfig(a.settings, sess.ModelName(a.DefaultModelName()))
	if !ok {
		return cllm.Pricing{}
	}
	return cfg.Pricing
}

func resolveWorkspaceRoot(dir string) (string, error) {
	root, err := util.ResolveWorkspaceRoot(dir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	return root, nil
}

func (s *SkillRegistry) Load(workspace string, plugins []config.DiscoveredPlugin) error {
	if s == nil {
		return nil
	}
	res := tools.ResolveSkills(config.SkillSources(workspace, plugins))
	s.entries, s.shadowed, s.findings = res.Entries, res.Shadowed, res.Findings
	return nil
}

func (s *SkillRegistry) Entries() []tools.SkillEntry {
	if s == nil || len(s.entries) == 0 {
		return nil
	}
	cloned := make([]tools.SkillEntry, len(s.entries))
	copy(cloned, s.entries)
	return cloned
}

func (s *SkillRegistry) NewTool() *tools.SkillTool {
	if s == nil {
		return tools.NewSkillFromEntries(nil)
	}
	return tools.NewSkillFromEntries(s.entries)
}

func (s *SubAgentRegistry) Load(workspace string, plugins []config.DiscoveredPlugin) error {
	if s == nil {
		return nil
	}
	res, err := tools.ResolveAgentDefs(config.AgentDefSources(workspace, plugins))
	if err != nil {
		return fmt.Errorf("load agent defs: %w", err)
	}
	s.userDefs, s.shadowed, s.findings = res.Defs, res.Shadowed, res.Findings
	return nil
}

func (s *SubAgentRegistry) Definitions() []subagent.Def {
	if s == nil || len(s.userDefs) == 0 {
		return nil
	}
	cloned := make([]subagent.Def, len(s.userDefs))
	copy(cloned, s.userDefs)
	return cloned
}

// Plugins returns the plugin inventory this runtime was assembled with.
func (a *AgentApp) Plugins() PluginSnapshot {
	if a == nil {
		return PluginSnapshot{}
	}
	return a.plugins
}

func (a *AgentApp) ListSessions() ([]session.ItemSummary, error) {
	if a == nil || a.sessionManager == nil {
		return nil, fmt.Errorf("session store is not initialized")
	}
	return a.sessionManager.List()
}

func (r *LLMClientCache) Get(modelName string) (cllm.LLMClient, error) {
	if r == nil {
		return nil, fmt.Errorf("settings store is not initialized")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if client, ok := r.clients[modelName]; ok {
		return client, nil
	}
	cfg, ok := FindModelConfig(r.settings, modelName)
	if !ok {
		return nil, fmt.Errorf("model not found: %q", modelName)
	}
	client, err := r.build(cfg)
	if err != nil {
		return nil, err
	}
	r.clients[modelName] = client
	return client, nil
}

// build makes the client for one model entry. The transport decides where the
// prompt goes, and the two paths never substitute for one another: a managed
// entry that cannot authenticate fails rather than quietly calling a provider
// with some other credential.
func (r *LLMClientCache) build(cfg ModelConfig) (cllm.LLMClient, error) {
	if serverURL := r.managedServerURL; serverURL != "" {
		if r.managedToken == nil {
			return nil, fmt.Errorf("model %q is served by %s, which this surface cannot authenticate to",
				cfg.Name, serverURL)
		}
		// Resolved once here so a model that cannot authenticate fails at
		// selection rather than at the first prompt, and again per request
		// below so a session outlasting its access token keeps working.
		if _, err := r.managedToken(serverURL); err != nil {
			return nil, fmt.Errorf("model %q: %w", cfg.Name, err)
		}
		return llmremote.NewClient(llmremote.Config{
			ServerURL:     serverURL,
			TokenFunc:     func() (string, error) { return r.managedToken(serverURL) },
			TaskRunID:     r.managedTaskRunID,
			Model:         cfg.ProviderModel,
			ContextWindow: cfg.ContextWindow,
			Surface:       r.surface,
			CallTimeout:   time.Duration(cfg.CallTimeout) * time.Second,
		}), nil
	}

	// A local runtime has no credential, and demanding one would make the
	// provider unusable without inventing a fake key.
	if cfg.APIKey == "" && config.LLMProviderNeedsAPIKey(cfg.Provider) {
		return nil, fmt.Errorf("api_key is required for model %q in settings.yaml", cfg.Name)
	}
	client, err := llm.NewClient(llm.Config{
		Provider:      cfg.Provider,
		APIKey:        cfg.APIKey,
		BaseURL:       cfg.BaseURL,
		Model:         cfg.ProviderModel,
		ContextWindow: cfg.ContextWindow,
		MaxTokens:     cfg.MaxTokens,
		Reasoning:     cfg.Reasoning,
		CacheControl:  cfg.CacheControl,
		Integration:   cfg.Integration,
		Vision:        cfg.Vision,
		Surface:       r.surface,
		KeepAlive:     cfg.KeepAlive,
		CallTimeout:   time.Duration(cfg.CallTimeout) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("model %q: %w", cfg.Name, err)
	}
	return client, nil
}

// promptCapabilities reports what this surface can actually do, so the prompt
// describes the tools the agent was given rather than the ones it might have.
func (a *AgentApp) promptCapabilities() PromptCapabilities {
	return PromptCapabilities{Artifacts: a.artifactPublisher != nil}
}

func (a *AgentApp) buildToolRegistry(client cllm.LLMClient) (cllm.ToolRegistry, error) {
	registry := cllm.NewToolRegistry()
	registry.AppendTools(buildBaseTools(client, a.workspaceRoot, a.skillsRegistry.NewTool(), a.Sandbox(), a.artifactPublisher, a.jobs)...)
	if a.mcpManager != nil {
		if reg := a.mcpManager.Registry(); reg != nil {
			registry.AppendTools(tools.GatewayTools(reg)...)
		}
	}
	agentTypes := BuildAgentTypes(registry, a.subagentsRegistry.Definitions())
	runner, err := tools.NewDefaultSubAgentRunner(client, a.policy, func(modelName string) (cllm.LLMClient, error) {
		return a.llmClients.Get(modelName)
	}, tools.WithSubAgentHooks(a.hooks), tools.WithSubAgentTraceFactory(a.newSubAgentTrace),
		tools.WithSubAgentSessionFactory(a.newSubAgentSession),
		tools.WithSubAgentMaxParallelTools(config.ResolveMaxParallelTools(a.settings.Agent)))
	if err == nil {
		taskTool, err := tools.NewTask(runner, agentTypes)
		if err == nil {
			if a.jobs != nil {
				taskTool = taskTool.WithJobs(a.jobs, a.workspaceRoot)
			}
			registry.AppendTools(taskTool)
		}
	}
	// After BuildAgentTypes like Task, so subagents never see the job tools:
	// a job must be owned by a session the user can still reach.
	if a.jobs != nil {
		registry.AppendTools(
			tools.NewJobList(a.jobs), tools.NewJobOutput(a.jobs), tools.NewJobStop(a.jobs),
			tools.NewMonitor(a.workspaceRoot).WithSandbox(a.Sandbox()).WithJobs(a.jobs),
		)
	}
	return registry, nil
}

// newSubAgentTrace opens a child trace linked to the immediate parent run.
// When the parent trace is unavailable, no child trace is created either: a
// trace with a missing link would misrepresent the relationship, and tracing
// must never become a reason for a subagent to fail.
//
// sessionID is the parent's, so a subagent run lands in the traces directory
// of a session that still exists and is_subagent tells the two apart.
func (a *AgentApp) newSubAgentTrace(ctx context.Context, sessionID string, opts tools.SubAgentRunOpts) tools.SubAgentTrace {
	parent := traceRunFromContext(ctx)
	if parent.runID == "" {
		// A background subagent runs on a manager-owned context that carries
		// only the explicit core-level provenance, not this package's key.
		parent.runID = agent.RunIDFromCtx(ctx)
	}
	if parent.runID == "" {
		return nil
	}
	modelName := parent.modelName
	if opts.Model != "" {
		modelName = opts.Model
	}
	// Fail-open for the same reason as the parent run's recorder above.
	runID, _ := util.NewPublicID()
	return trace.NewRecorder(sessionstore.SessionTracesDir(a.sessionManager.Dir(), sessionID), trace.Meta{
		RunID:            runID,
		ParentRunID:      parent.runID,
		ParentToolCallID: agent.ToolCallFromCtx(ctx),
		SessionID:        sessionID,
		Workspace:        a.workspaceRoot,
		Model:            modelName,
		IsSubagent:       true,
		Sandbox:          a.sandboxInfo(),
		PromptLayers: []agent.PromptLayer{
			{Name: "subagent_system_prompt", Chars: len(opts.SystemPrompt)},
		},
	})
}

func (a *AgentApp) toolRegistry(modelName string, client cllm.LLMClient) (cllm.ToolRegistry, error) {
	if a == nil {
		return cllm.ToolRegistry{}, fmt.Errorf("app is nil")
	}
	a.toolRegistriesMu.Lock()
	defer a.toolRegistriesMu.Unlock()
	if registry, ok := a.toolRegistries[modelName]; ok {
		return registry, nil
	}
	registry, err := a.buildToolRegistry(client)
	if err != nil {
		return cllm.ToolRegistry{}, err
	}
	a.toolRegistries[modelName] = registry
	return registry, nil
}

func DefaultModelName(settings config.Settings) string {
	cfg, ok := DefaultModelConfig(settings)
	if !ok {
		return ""
	}
	return cfg.Name
}

// DefaultModelConfig is the model a new session starts with: the one
// default_model names, or the first entry when it names none.
//
// A default_model matching nothing falls through to the first entry rather than
// failing here, because a model picker that returns nothing is worse than one
// that returns the wrong first choice. `buildmax doctor` reports the mismatch.
func DefaultModelConfig(settings config.Settings) (ModelConfig, bool) {
	if len(settings.Models) == 0 {
		return ModelConfig{}, false
	}
	if name := settings.DefaultModel; name != "" {
		for _, entry := range settings.Models {
			cfg := toModelConfig(entry)
			if cfg.Name == name || cfg.ProviderModel == name {
				return cfg, true
			}
		}
	}
	return toModelConfig(settings.Models[0]), true
}

func FindModelConfig(settings config.Settings, name string) (ModelConfig, bool) {
	if name == "" {
		return DefaultModelConfig(settings)
	}
	for _, entry := range settings.Models {
		cfg := toModelConfig(entry)
		if cfg.Name == name || cfg.ProviderModel == name {
			return cfg, true
		}
	}
	if len(settings.Models) == 0 {
		cfg, ok := DefaultModelConfig(settings)
		if ok && cfg.Name == name {
			return cfg, true
		}
	}
	return ModelConfig{}, false
}

// ModelConfigFromEntry resolves one settings.yaml model entry. Surfaces use it
// to describe a model without building a client for it.
func ModelConfigFromEntry(entry config.ModelEntry) ModelConfig { return toModelConfig(entry) }

func toModelConfig(entry config.ModelEntry) ModelConfig {
	name := entry.Name
	if name == "" {
		name = entry.Model
	}
	pricing, err := config.ResolvePricing(entry.Pricing)
	var pricingErr string
	if err != nil {
		pricingErr = err.Error()
	}
	return ModelConfig{
		Name:          name,
		ProviderModel: entry.Model,
		BaseURL:       entry.APIURL,
		APIKey:        entry.APIKey,
		ContextWindow: entry.ContextWindow,
		CallTimeout:   entry.CallTimeout,
		MaxTokens:     entry.MaxTokens,
		Reasoning:     entry.Reasoning,
		CacheControl:  config.ResolveCacheControl(entry.CacheControl),
		Pricing:       pricing,
		PricingErr:    pricingErr,
		Integration:   entry.Integration,
		Vision:        entry.Vision,
		KeepAlive:     entry.KeepAlive,
		Provider:      entry.Provider,
	}
}
