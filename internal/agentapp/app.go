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
	// Surface labels managed calls for correlation, e.g. "cli" or "desktop".
	Surface string
}

// ManagedTokenFunc returns the BuildMax credential to use for serverURL. It is
// expected to refuse when the stored login belongs to a different server.
type ManagedTokenFunc func(serverURL string) (string, error)

type AgentApp struct {
	workspaceRoot        string
	settings             config.Settings
	llmClients           *LLMClientCache
	toolRegistriesMu     sync.Mutex
	toolRegistries       map[string]cllm.ToolRegistry
	mcpManager           *MCPManager
	skillsRegistry       *SkillRegistry
	subagentsRegistry    *SubAgentRegistry
	sessionManager       *SessionManager
	modelMu              sync.Mutex
	defaultModelOverride string
	policy               agent.ToolPolicy
	hooks                agent.HookRunner
	sandbox              agent.SandboxView
	sandboxManager       *sandbox.Manager
	sandboxResolved      config.SandboxResolution
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

func NewAgentApp(cfg AppConfig) (*AgentApp, error) {
	workspaceRoot, err := resolveWorkspaceRoot(cfg.WorkspaceDir)
	if err != nil {
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
		workspaceRoot:     workspaceRoot,
		settings:          settings,
		toolRegistries:    make(map[string]cllm.ToolRegistry),
		sessionManager:    &SessionManager{dir: config.SessionsDir()},
		skillsRegistry:    &SkillRegistry{},
		subagentsRegistry: &SubAgentRegistry{},
		policy:            cfg.Policy,
		sandbox:           sandboxView,
		sandboxManager:    sandboxManager,
		sandboxResolved:   sandboxResolved,
	}
	app.llmClients = &LLMClientCache{
		settings:     app.settings,
		managedToken: cfg.ManagedToken,
		surface:      cfg.Surface,
		clients:      make(map[string]cllm.LLMClient),
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
		entries = append(entries, ToolEntry{Name: t.Name(), Description: t.Description()})
	}
	return entries
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
	systemPrompt := BuildEffectiveSystemPrompt(a.workspaceRoot, modelName)
	if sess.CompactionSummary != "" {
		systemPrompt += "\n\n<context_compaction>\n" + sess.CompactionSummary + "\n</context_compaction>"
	}
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

func (a *AgentApp) RunPrompt(ctx context.Context, sess *SessionContext, prompt string, stream cllm.StreamSink, approval agent.ApprovalHandler, eventSink func(agent.Event)) (RunResult, error) {
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

	// Durable run trace: one JSONL file per run, attached at this single
	// chokepoint so CLI/TUI, Desktop, eval, and the worker all produce traces
	// with no per-surface code. Fail-open — a nil recorder is a no-op.
	var recorder *trace.Recorder
	if config.TraceEnabled() {
		recorder = trace.NewRecorder(config.TracesDir(), trace.Meta{
			RunID:     util.NewPrefixedID("rt"),
			SessionID: sess.ID,
			Workspace: a.workspaceRoot,
			Model:     modelName,
			Sandbox:   a.sandboxInfo(),
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

	systemPrompt := BuildEffectiveSystemPrompt(a.workspaceRoot, modelName)
	if sess.CompactionSummary != "" {
		systemPrompt += "\n\n<context_compaction>\n" + sess.CompactionSummary + "\n</context_compaction>"
	}

	reply, stats, err := agent.RunLoop(ctx, agent.RunLoopOpts{
		LLMClient:    client,
		SystemPrompt: systemPrompt,
		ToolRegistry: registry,
		MaxIter:      agent.DefaultMaxIterations,
		History:      sess,
		StreamSink:   stream,
		Policy:       a.policy,
		Approval:     approval,
		Compactor:    NewLLMCompactor(client),
		EventSink:    teeEventSink(recorder.Record, eventSink),
		Hooks:        a.hooks,
		SessionID:    sess.ID,
		Workspace:    a.workspaceRoot,
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
	s.entries = tools.DiscoverSkillEntries(config.SkillSearchPaths(workspace))
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
	defs, err := tools.LoadAgentDefsFromPaths(config.AgentDefsSearchPaths(workspace))
	if err != nil {
		return fmt.Errorf("load agent defs: %w", err)
	}
	s.userDefs = defs
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
		if cfg.TeamID == "" {
			return nil, fmt.Errorf("team_id is required for model %q in settings.yaml", cfg.Name)
		}
		token, err := r.managedToken(cfg.ServerURL)
		if err != nil {
			return nil, fmt.Errorf("model %q: %w", cfg.Name, err)
		}
		return llmremote.NewClient(llmremote.Config{
			ServerURL:     cfg.ServerURL,
			Token:         token,
			TeamID:        cfg.TeamID,
			Alias:         cfg.ProviderModel,
			ContextWindow: cfg.ContextWindow,
			Surface:       r.surface,
			CallTimeout:   time.Duration(cfg.CallTimeout) * time.Second,
		}), nil
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for model %q in settings.yaml", cfg.Name)
	}
	return llm.NewClient(llm.Config{
		APIKey:        cfg.APIKey,
		BaseURL:       cfg.BaseURL,
		Model:         cfg.ProviderModel,
		ContextWindow: cfg.ContextWindow,
		CallTimeout:   time.Duration(cfg.CallTimeout) * time.Second,
	}), nil
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
		Transport:     entry.Transport,
		ServerURL:     entry.ServerURL,
		TeamID:        entry.TeamID,
	}
}
