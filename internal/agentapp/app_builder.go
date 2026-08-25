package agentapp

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/infra/hook"
)

// resolvedAgentAppConfig is the immutable input to runtime construction. File,
// environment, plugin, hook, and sandbox precedence are all settled before a
// resource-owning AgentApp exists.
type resolvedAgentAppConfig struct {
	workspaceRoot string
	settings      config.Settings
	plugins       PluginSnapshot
	loadedPlugins []config.DiscoveredPlugin
	hooks         corehook.Config
	sandbox       config.SandboxResolution
}

func resolveAgentAppConfig(cfg AppConfig) (resolvedAgentAppConfig, error) {
	workspaceRoot, err := resolveWorkspaceRoot(cfg.WorkspaceDir)
	if err != nil {
		return resolvedAgentAppConfig{}, err
	}
	if err := ValidateAdditionalSystemPrompt(cfg.AdditionalSystemPrompt); err != nil {
		return resolvedAgentAppConfig{}, err
	}
	settings, err := config.LoadSettings()
	if err != nil {
		return resolvedAgentAppConfig{}, fmt.Errorf("load settings: %w", err)
	}
	if len(cfg.ModelEntries) > 0 {
		settings.Models = append([]config.ModelEntry(nil), cfg.ModelEntries...)
		settings.DefaultModel = cfg.DefaultModel
	}

	plugins := discoverPlugins()
	plugins.resolveBase(context.Background())
	loadedPlugins := plugins.Loadable()
	workspaceHooks, err := config.LoadWorkspaceHooks(workspaceRoot)
	if err != nil {
		return resolvedAgentAppConfig{}, fmt.Errorf("load workspace hooks: %w", err)
	}
	pluginHooks := config.ResolvePluginHooks(loadedPlugins)
	plugins.addFindings(pluginHooks.Findings...)

	policySandbox, err := config.LoadPolicySandbox()
	if err != nil {
		return resolvedAgentAppConfig{}, fmt.Errorf("load policy sandbox: %w", err)
	}
	surface := cfg.SandboxSurface
	if surface == "" {
		surface = config.SandboxSurfaceCLI
	}

	return resolvedAgentAppConfig{
		workspaceRoot: workspaceRoot,
		settings:      settings,
		plugins:       plugins,
		loadedPlugins: loadedPlugins,
		hooks:         config.MergeHooks(settings.Hooks, pluginHooks.Config, workspaceHooks),
		sandbox:       config.ResolveSandbox(settings.Sandbox, policySandbox, surface),
	}, nil
}

// buildAgentApp opens runtime resources from already-resolved configuration.
// Any partial construction is closed before an error is returned.
func buildAgentApp(cfg AppConfig, resolved resolvedAgentAppConfig) (_ *AgentApp, err error) {
	sandboxManager, err := buildSandboxManager(resolved.sandbox, resolved.workspaceRoot)
	if err != nil {
		return nil, err
	}

	app := &AgentApp{
		workspaceRoot:          resolved.workspaceRoot,
		settings:               resolved.settings,
		toolRegistries:         make(map[string]cllm.ToolRegistry),
		sessionManager:         NewSessionManager(config.SessionsDir()),
		skillsRegistry:         &SkillRegistry{},
		subagentsRegistry:      &SubAgentRegistry{},
		policy:                 NewConfiguredPolicy(config.ResolvePermissions(resolved.settings.Tools), cfg.Policy),
		additionalSystemPrompt: cfg.AdditionalSystemPrompt,
		artifactPublisher:      cfg.ArtifactPublisher,
		sandbox:                agent.SandboxView(sandboxManager),
		sandboxManager:         sandboxManager,
		sandboxResolved:        resolved.sandbox,
		plugins:                resolved.plugins,
	}
	complete := false
	defer func() {
		if !complete {
			_ = app.Close()
		}
	}()

	app.llmClients = &LLMClientCache{
		settings:         app.settings,
		managedServerURL: cfg.ManagedServerURL,
		managedToken:     cfg.ManagedToken,
		managedTaskRunID: cfg.ManagedTaskRunID,
		surface:          cfg.Surface,
		clients:          make(map[string]cllm.LLMClient),
	}
	if cfg.EnableMCP {
		mcpResolution, resolveErr := config.ResolveMCPConfig(app.workspaceRoot, resolved.loadedPlugins)
		if resolveErr != nil {
			return nil, fmt.Errorf("load mcp config: %w", resolveErr)
		}
		app.plugins.addFindings(mcpResolution.Findings...)
		app.plugins.addShadowed(mcpResolution.Shadowed...)
		app.mcpManager, err = NewMCPManager(context.Background(), mcpResolution.Config)
		if err != nil {
			return nil, err
		}
	}

	hookDeps := hook.Deps{LLMCaller: &llmCaller{cache: app.llmClients, defaultModel: app.DefaultModelName}}
	if app.mcpManager != nil {
		hookDeps.MCPCaller = &mcpCaller{m: app.mcpManager}
	}
	app.hooks = NewHookManager(resolved.hooks, hook.NewDriverRegistry(hookDeps))

	if err = app.skillsRegistry.Load(app.workspaceRoot, resolved.loadedPlugins); err != nil {
		return nil, err
	}
	if err = app.subagentsRegistry.Load(app.workspaceRoot, resolved.loadedPlugins); err != nil {
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
	complete = true
	return app, nil
}
