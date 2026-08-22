package agentapp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/infra/hook"
	llm "github.com/gougoujiang/buildmax/internal/infra/llm"
	"github.com/gougoujiang/buildmax/internal/infra/llmremote"
	"github.com/gougoujiang/buildmax/internal/infra/sandbox"
	"github.com/gougoujiang/buildmax/internal/infra/trace"
	tools "github.com/gougoujiang/buildmax/internal/tool"
	"github.com/gougoujiang/buildmax/internal/util"
)

type AppConfig struct {
	WorkspaceDir string
	EnableMCP    bool
	// ModelEntries overrides settings.yaml models for this AgentApp. Workers use
	// this to receive the server's resolved model without writing credentials to
	// a run directory that is later persisted as an artifact.
	ModelEntries []config.ModelEntry
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
	sessionManager         *SessionManager
	modelMu                sync.Mutex
	defaultModelOverride   string
	policy                 agent.ToolPolicy
	hooks                  agent.HookRunner
	sandbox                agent.SandboxView
	sandboxManager         *sandbox.Manager
	sandboxResolved        config.SandboxResolution
	additionalSystemPrompt string
	grantsMu               sync.Mutex
	grants                 map[string]*agent.SessionGrants
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
	entries []tools.SkillEntry
}

type SubAgentRegistry struct {
	userDefs []tools.SubAgentDef
}

type LLMClientCache struct {
	settings config.Settings
	// managedToken supplies the BuildMax credential for a managed entry. Nil
	// means this surface does not offer managed inference — the worker and the
	// evaluation harness deliberately do not.
	managedToken ManagedTokenFunc
	// managedTaskRunID scopes managed calls to one task run. Empty means they
	// are team-scoped.
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
	ContextTokens         int
	ContextWindow         int
	SessionID             string
	Workspace             string
	ModelName             string
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
}

type TurnFinalizeResult struct {
	Title            string
	PromptTokens     int
	CompletionTokens int
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
	// PromptCache caches the stable prefix of a request.
	PromptCache bool
	// Vision says this model accepts image input.
	Vision bool
	// Provider is the wire protocol a direct entry speaks. Empty means
	// config.LLMProviderOpenAICompatible. A managed entry ignores it: the
	// operator's catalog decides which protocol serves the call.
	Provider string
	// Transport is config.TransportDirect or config.TransportBuildMax. Empty
	// means direct.
	Transport string
	// ServerURL and TeamID are set on a managed entry. ProviderModel then holds
	// the team alias rather than a provider's model identifier.
	ServerURL string
	TeamID    string
}

// IsManaged reports whether this model calls a BuildMax gateway.
func (c ModelConfig) IsManaged() bool { return c.Transport == config.TransportBuildMax }

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
		return sess.AdditionalSystemPrompt
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
		settings.Models = append([]config.ModelEntry(nil), cfg.ModelEntries...)
	}
	wsHooks, err := config.LoadWorkspaceHooks(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("load workspace hooks: %w", err)
	}
	mergedHooks := config.MergeHooks(settings.Hooks, wsHooks)

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
		sessionManager:         &SessionManager{dir: config.SessionsDir()},
		skillsRegistry:         &SkillRegistry{},
		subagentsRegistry:      &SubAgentRegistry{},
		policy:                 NewConfiguredPolicy(config.ResolvePermissions(settings.Tools), cfg.Policy),
		additionalSystemPrompt: cfg.AdditionalSystemPrompt,
		sandbox:                sandboxView,
		sandboxManager:         sandboxManager,
		sandboxResolved:        sandboxResolved,
	}
	app.llmClients = &LLMClientCache{
		settings:         app.settings,
		managedToken:     cfg.ManagedToken,
		managedTaskRunID: cfg.ManagedTaskRunID,
		surface:          cfg.Surface,
		clients:          make(map[string]cllm.LLMClient),
	}
	if cfg.EnableMCP {
		mcpCfg, err := config.LoadMCPConfigForWorkspace(app.workspaceRoot)
		if err != nil {
			return nil, fmt.Errorf("load mcp config: %w", err)
		}
		app.mcpManager, err = NewMCPManager(context.Background(), mcpCfg)
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

	if err := app.skillsRegistry.Load(app.workspaceRoot); err != nil {
		return nil, err
	}
	if err := app.subagentsRegistry.Load(app.workspaceRoot); err != nil {
		return nil, err
	}
	return app, nil
}

func (a *AgentApp) Close() error {
	if a == nil {
		return nil
	}
	var firstErr error
	if a.mcpManager != nil {
		if err := a.mcpManager.Close(); err != nil {
			firstErr = err
		}
	}
	if a.sandboxManager != nil {
		if err := a.sandboxManager.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
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
func (a *AgentApp) AgentDefs() []tools.SubAgentDef {
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
	mcpCfg, err := config.LoadMCPConfigForWorkspace(a.workspaceRoot)
	if err != nil {
		return MCPStatus{}, fmt.Errorf("load mcp config: %w", err)
	}
	if err := a.mcpManager.Refresh(ctx, mcpCfg); err != nil {
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
		sess = a.sessionManager.Create(a.DefaultModelName())
	} else {
		sess, err = a.sessionManager.Load(sessionID, a.DefaultModelName())
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
	sess = NewSessionContext(session.NewSessionFromData(sessionID, "", time.Now(), nil, 0, 0), a.DefaultModelName())
	a.fireSessionLifecycle(agent.HookSessionStart, sess, nil)
	return sess, nil
}

// CloseSession fires the SessionEnd hook for a finished session. Sessions
// persist on disk; this is the explicit signal for hooks/audit that the
// caller is done with that session. Safe to call with a nil session.
func (a *AgentApp) CloseSession(sess *SessionContext) {
	if a == nil || sess == nil {
		return
	}
	a.fireSessionLifecycle(agent.HookSessionEnd, sess, nil)
}

func (a *AgentApp) fireSessionLifecycle(event agent.HookEvent, sess *SessionContext, stats *agent.RunStats) {
	if a == nil || a.hooks == nil || sess == nil {
		return
	}
	in := agent.HookInput{
		Event:     event,
		SessionID: sess.ID,
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
		sess = NewSessionContext(session.NewSession(""), a.DefaultModelName())
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
	systemPrompt := BuildEffectiveSystemPrompt(a.workspaceRoot, modelName, a.effectiveAdditionalPrompt(sess)) + agent.RenderCompactionBlock(sess.CompactionSummary)
	contextTokens := agent.EstimateMessageTokens(cllm.Message{Role: "system", Content: systemPrompt}) + agent.EstimateTokens(sess.HistoryMessages())
	return RunStatus{
		ContextTokens:         contextTokens,
		ContextWindow:         contextWindow,
		TotalPromptTokens:     sess.PromptTokens,
		TotalCompletionTokens: sess.CompletionTokens,
	}
}

func (a *AgentApp) resolveRunContext(sess *SessionContext) (*SessionContext, string, cllm.LLMClient, error) {
	if a == nil {
		return nil, "", nil, fmt.Errorf("app is nil")
	}
	if sess == nil {
		sess = NewSessionContext(session.NewSession(""), a.DefaultModelName())
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

func (a *AgentApp) RunPrompt(ctx context.Context, sess *SessionContext, prompt string, opts RunPromptOpts) (RunResult, error) {
	sess, modelName, client, err := a.resolveRunContext(sess)
	if err != nil {
		return RunResult{}, err
	}
	registry, err := a.toolRegistry(modelName, client)
	if err != nil {
		return RunResult{}, err
	}
	start := time.Now()
	ctx = session.CtxWithSessionID(ctx, sess.ID)
	// NoteWrite and TodoWrite reach the session through the context: the tool registry is
	// cached per model and shared across sessions, so a tool must not hold one.
	ctx = agent.CtxWithNoteStore(ctx, sess)

	// Resolved before the trace opens, because the trace reports which prompt layers this run
	// loaded and a run that ends early still has to be able to say.
	extraPrompt := a.effectiveAdditionalPrompt(sess)
	systemPrompt, promptLayers := BuildSystemPromptWithLayers(a.workspaceRoot, modelName, extraPrompt)
	sess.AdditionalSystemPrompt = extraPrompt

	// Durable run trace: one JSONL file per run, attached at this single
	// chokepoint so CLI/TUI, Desktop, eval, and the worker all produce traces
	// with no per-surface code. Fail-open — a nil recorder is a no-op.
	var recorder *trace.Recorder
	if config.TraceEnabled() {
		recorder = trace.NewRecorder(config.TracesDir(), trace.Meta{
			RunID:        util.NewPrefixedID("rt"),
			SessionID:    sess.ID,
			Workspace:    a.workspaceRoot,
			Model:        modelName,
			Sandbox:      a.sandboxInfo(),
			PromptLayers: promptLayers,
		})
		defer recorder.Close()
	}

	// Give hooks a chance to inspect / reject the prompt before it enters
	// history or the LLM. A block short-circuits the turn: the prompt is
	// not appended, the LLM is not called, and the user receives the
	// hook's reason as the reply.
	promptHook := a.hooks.Run(ctx, agent.HookInput{
		Event:     agent.HookUserPromptSubmit,
		SessionID: sess.ID,
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
			TotalPromptTokens:     sess.PromptTokens,
			TotalCompletionTokens: sess.CompletionTokens,
			ContextTokens:         status.ContextTokens,
			ContextWindow:         status.ContextWindow,
			SessionID:             sess.ID,
			Workspace:             a.workspaceRoot,
			ModelName:             modelName,
			TraceID:               recorder.RunID(),
			TracePath:             recorder.Path(),
		}, nil
	}

	if err := sess.Append(cllm.Message{Role: "user", Content: prompt}); err != nil {
		return RunResult{}, err
	}

	// No compaction block here: RunLoop reads the stored summary through
	// CompactionHistory and renders it itself. Appending it here as well put two
	// <context_compaction> blocks in the prompt after the first in-run compaction.
	reply, stats, err := agent.RunLoop(ctx, agent.RunLoopOpts{
		LLMClient:    client,
		SystemPrompt: systemPrompt,
		ToolRegistry: registry,
		MaxIter:      agent.DefaultMaxIterations,
		History:      sess,
		StreamSink:   opts.Stream,
		Policy:       a.policy,
		Approval:     opts.Approval,
		PendingInput: opts.Pending,
		Grants:       a.grantsFor(sess.ID),

		MaxParallelTools: config.ResolveMaxParallelTools(a.settings.Agent),
		Compactor:        NewLLMCompactor(client),
		Checkpointer:     NewNoteCheckpointer(client),
		Invariants:       agent.ExtractInvariants(extraPrompt),
		EventSink:        teeEventSink(recorder.Record, opts.EventSink),
		Hooks:            a.hooks,
		SessionID:        sess.ID,
		Workspace:        a.workspaceRoot,
	})
	// Failed runs still leave a complete trace (RunLoop emits run_end with the
	// error), so carry TraceID out even on the error paths — a failed run is
	// exactly when the caller most needs to point a viewer at the file.
	if err != nil {
		return RunResult{SessionID: sess.ID, TraceID: recorder.RunID(), TracePath: recorder.Path()}, fmt.Errorf("agent: %w", err)
	}
	if _, err := a.finalizeTurn(sess, client, stats); err != nil {
		return RunResult{SessionID: sess.ID, TraceID: recorder.RunID(), TracePath: recorder.Path()}, err
	}
	status := a.estimateRunStatus(sess, modelName, client.ContextWindow())
	return RunResult{
		Reply:                 reply,
		Duration:              time.Since(start),
		ToolCalls:             stats.ToolCalls,
		PromptTokens:          stats.PromptTokens,
		CompletionTokens:      stats.CompletionTokens,
		TotalPromptTokens:     sess.PromptTokens,
		TotalCompletionTokens: sess.CompletionTokens,
		ContextTokens:         status.ContextTokens,
		ContextWindow:         status.ContextWindow,
		SessionID:             sess.ID,
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
	return a.sessionManager.Finalize(context.Background(), client, sess, a.workspaceRoot, stats)
}

func resolveWorkspaceRoot(dir string) (string, error) {
	root, err := util.ResolveWorkspaceRoot(dir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	return root, nil
}

func (s *SkillRegistry) Load(workspace string) error {
	if s == nil {
		return nil
	}
	s.entries = tools.ResolveSkills(config.SkillSources(workspace, nil)).Entries
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

func (s *SubAgentRegistry) Load(workspace string) error {
	if s == nil {
		return nil
	}
	res, err := tools.ResolveAgentDefs(config.AgentDefSources(workspace, nil))
	if err != nil {
		return fmt.Errorf("load agent defs: %w", err)
	}
	s.userDefs = res.Defs
	return nil
}

func (s *SubAgentRegistry) Definitions() []tools.SubAgentDef {
	if s == nil || len(s.userDefs) == 0 {
		return nil
	}
	cloned := make([]tools.SubAgentDef, len(s.userDefs))
	copy(cloned, s.userDefs)
	return cloned
}

func (a *AgentApp) ListSessions() ([]session.SessionItem, error) {
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
	if cfg.IsManaged() {
		if r.managedToken == nil {
			return nil, fmt.Errorf("model %q uses transport %q, which this surface does not support",
				cfg.Name, config.TransportBuildMax)
		}
		// A run-scoped app calls as its task run, so it needs no team: the server
		// reads user and team from the run token. Any team on the entry is
		// dropped rather than sent, because the two identities select different
		// routes and the client refuses to hold both.
		teamID := cfg.TeamID
		if r.managedTaskRunID != "" {
			teamID = ""
		} else if teamID == "" {
			return nil, fmt.Errorf("team_id is required for model %q in settings.yaml", cfg.Name)
		}
		// Resolved once here so a model that cannot authenticate fails at
		// selection rather than at the first prompt, and again per request
		// below so a session outlasting its access token keeps working.
		if _, err := r.managedToken(cfg.ServerURL); err != nil {
			return nil, fmt.Errorf("model %q: %w", cfg.Name, err)
		}
		serverURL := cfg.ServerURL
		return llmremote.NewClient(llmremote.Config{
			ServerURL:     cfg.ServerURL,
			TokenFunc:     func() (string, error) { return r.managedToken(serverURL) },
			TeamID:        teamID,
			TaskRunID:     r.managedTaskRunID,
			Alias:         cfg.ProviderModel,
			ContextWindow: cfg.ContextWindow,
			Surface:       r.surface,
			CallTimeout:   time.Duration(cfg.CallTimeout) * time.Second,
		}), nil
	}

	if cfg.APIKey == "" {
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
		PromptCache:   cfg.PromptCache,
		Vision:        cfg.Vision,
		CallTimeout:   time.Duration(cfg.CallTimeout) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("model %q: %w", cfg.Name, err)
	}
	return client, nil
}

func (a *AgentApp) buildToolRegistry(client cllm.LLMClient) (cllm.ToolRegistry, error) {
	registry := cllm.NewToolRegistry()
	registry.AppendTools(buildBaseTools(client, a.workspaceRoot, a.skillsRegistry.NewTool(), a.Sandbox())...)
	if a.mcpManager != nil {
		if reg := a.mcpManager.Registry(); reg != nil {
			registry.AppendTools(tools.GatewayTools(reg)...)
		}
	}
	agentTypes := BuildAgentTypes(registry, a.subagentsRegistry.Definitions())
	runner, err := tools.NewDefaultSubAgentRunner(client, a.policy, func(modelName string) (cllm.LLMClient, error) {
		return a.llmClients.Get(modelName)
	}, tools.WithSubAgentHooks(a.hooks))
	if err == nil {
		taskTool, err := tools.NewTask(runner, agentTypes)
		if err == nil {
			registry.AppendTools(taskTool)
		}
	}
	return registry, nil
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

func DefaultModelConfig(settings config.Settings) (ModelConfig, bool) {
	if len(settings.Models) == 0 {
		return ModelConfig{}, false
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
	return ModelConfig{
		Name:          name,
		ProviderModel: entry.Model,
		BaseURL:       entry.APIURL,
		APIKey:        entry.APIKey,
		ContextWindow: entry.ContextWindow,
		CallTimeout:   entry.CallTimeout,
		MaxTokens:     entry.MaxTokens,
		Reasoning:     entry.Reasoning,
		PromptCache:   entry.PromptCache,
		Vision:        entry.Vision,
		Provider:      entry.Provider,
		Transport:     entry.Transport,
		ServerURL:     entry.ServerURL,
		TeamID:        entry.TeamID,
	}
}
