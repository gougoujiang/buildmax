package agentrun

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"buildmax/internal/config"
	"buildmax/internal/core/agent"
	"buildmax/internal/core/model"
	tools "buildmax/internal/execution/builtintool"
	llm "buildmax/internal/infra/llm"
	"buildmax/internal/infra/mcp"
	"buildmax/internal/session"
	"buildmax/internal/util"
)

// Runtime holds the reusable agent execution state for one workspace and session.
type Runtime struct {
	agent       *agent.Agent
	llmClient   *llm.Client
	session     *session.Session
	sessionsDir string
	workspace   string
	modelName   string

	mcpCloseOnce sync.Once
	mcpCleanup   func()
}

// OpenInput configures Runtime creation.
type OpenInput struct {
	WorkspaceDir  string
	SessionID     string
	ModelSelector string
	// EnableMCP, when true, loads mcp.json (CLI only) and registers LoadMcpTools + CallMcpTool.
	EnableMCP bool
}

// RunInput configures one agent run.
type RunInput struct {
	Prompt string
	Stream model.StreamSink
}

// RunOutput is the result of one agent run.
type RunOutput struct {
	Reply                 string
	Duration              time.Duration
	ToolCalls             int
	PromptTokens          int
	CompletionTokens      int
	TotalPromptTokens     int
	TotalCompletionTokens int
	SessionID             string
	Workspace             string
}

// Open creates a runtime bound to a workspace and session.
func Open(in OpenInput) (*Runtime, error) {
	cfg, modelName, err := config.EffectiveLLMWithSelector("", in.ModelSelector)
	if err != nil {
		return nil, err
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key required. Set %s", config.EnvKeyBuildmaxAPIKey)
	}

	workspaceDir, err := resolveWorkspaceDir(in.WorkspaceDir)
	if err != nil {
		return nil, err
	}
	ws, err := util.NewWorkspace(workspaceDir)
	if err != nil {
		slog.Error("create workspace", "err", err)
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	client := llm.NewClient(llm.Config{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model})
	a, mcpCleanup, err := buildRuntimeAgent(client, ws, workspaceDir, in.EnableMCP)
	if err != nil {
		return nil, err
	}
	sessionsDir, sess, err := openSessionState(in.SessionID)
	if err != nil {
		return nil, err
	}

	return &Runtime{
		agent:       a,
		llmClient:   client,
		session:     sess,
		sessionsDir: sessionsDir,
		workspace:   workspaceDir,
		modelName:   modelName,
		mcpCleanup:  mcpCleanup,
	}, nil
}

func resolveWorkspaceDir(workspaceDir string) (string, error) {
	if workspaceDir != "" {
		return workspaceDir, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		slog.Error("get working directory", "err", err)
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return dir, nil
}

func buildRuntimeAgent(client *llm.Client, ws *util.Workspace, workspaceDir string, enableMCP bool) (*agent.Agent, func(), error) {
	baseTools, registry, err := buildBaseTools(client, ws, config.SkillSearchPaths(workspaceDir))
	if err != nil {
		slog.Error("build base tools", "err", err)
		return nil, nil, err
	}
	mcpCleanup, err := enableMCPTools(workspaceDir, enableMCP, &baseTools, &registry)
	if err != nil {
		return nil, nil, err
	}
	agentTypes := buildAgentTypes(baseTools, registry, workspaceDir)
	runner, err := NewDefaultSubAgentRunner(client)
	if err != nil {
		slog.Error("create sub-agent runner", "err", err)
		return nil, nil, fmt.Errorf("create sub-agent runner: %w", err)
	}
	taskTool, err := tools.NewTask(runner, agentTypes)
	if err != nil {
		slog.Error("create task tool", "err", err)
		return nil, nil, fmt.Errorf("create task tool: %w", err)
	}
	registry.AppendTools(taskTool)
	return agent.NewAgent(client, registry, agent.WithSystemPrompt(buildEffectiveSystemPrompt(workspaceDir))), mcpCleanup, nil
}

func openSessionState(sessionID string) (string, *session.Session, error) {
	sessionsDir := config.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		slog.Error("create sessions dir", "err", err)
		return "", nil, fmt.Errorf("create sessions dir: %w", err)
	}
	sess, err := loadOrCreateSession(sessionsDir, sessionID)
	if err != nil {
		return "", nil, err
	}
	return sessionsDir, sess, nil
}

func (r *Runtime) Agent() *agent.Agent {
	if r == nil {
		return nil
	}
	return r.agent
}

func (r *Runtime) LLMClient() *llm.Client {
	if r == nil {
		return nil
	}
	return r.llmClient
}

func (r *Runtime) Session() *session.Session {
	if r == nil {
		return nil
	}
	return r.session
}

func (r *Runtime) SessionsDir() string {
	if r == nil {
		return ""
	}
	return r.sessionsDir
}

func (r *Runtime) WorkspaceDir() string {
	if r == nil {
		return ""
	}
	return r.workspace
}

func (r *Runtime) ModelName() string {
	if r == nil {
		return ""
	}
	return r.modelName
}

func enableMCPTools(workspaceDir string, enable bool, baseTools *[]model.Tool, registry *model.ToolRegistry) (func(), error) {
	if !enable {
		return nil, nil
	}
	mcpCfg, err := config.LoadMCPConfigForWorkspace(workspaceDir)
	if err != nil {
		slog.Error("load mcp config", "err", err)
		return nil, err
	}
	if mcpCfg == nil || len(mcpCfg.MCPServers) == 0 {
		return nil, nil
	}
	ctx := context.Background()
	reg, err := mcp.NewRegistry(ctx, toMCPConfig(mcpCfg), nil)
	if err != nil {
		slog.Error("mcp registry", "err", err)
		return nil, fmt.Errorf("mcp: %w", err)
	}
	mcpTools := tools.GatewayTools(reg)
	*baseTools = append(*baseTools, mcpTools...)
	registry.AppendTools(mcpTools...)
	return func() { _ = reg.Close() }, nil
}

func buildEffectiveSystemPrompt(workspaceDir string) string {
	effectivePrompt := agent.DefaultSystemPrompt
	if extra, err := agent.ReadAgentsMd(workspaceDir); err != nil {
		slog.Warn("read AGENTS.md", "err", err)
	} else if extra != "" {
		effectivePrompt = effectivePrompt + "\n\n" + extra
	}
	return effectivePrompt
}

// Close releases resources held by the runtime (e.g. MCP client sessions when EnableMCP was used).
// Safe to call multiple times.
func (r *Runtime) Close() {
	if r == nil {
		return
	}
	r.mcpCloseOnce.Do(func() {
		if r.mcpCleanup != nil {
			r.mcpCleanup()
			r.mcpCleanup = nil
		}
	})
}

func toMCPConfig(cfg *config.MCPConfigRoot) *mcp.ConfigRoot {
	if cfg == nil {
		return nil
	}
	out := &mcp.ConfigRoot{MCPServers: make(map[string]mcp.ServerConfig, len(cfg.MCPServers))}
	for id, server := range cfg.MCPServers {
		out.MCPServers[id] = mcp.ServerConfig{
			Type:    server.Type,
			Command: server.Command,
			Args:    server.Args,
			Env:     server.Env,
			URL:     server.URL,
		}
	}
	return out
}

// RunPrompt executes one user prompt, persists the session, and returns run metadata.
func (r *Runtime) RunPrompt(ctx context.Context, in RunInput) (RunOutput, error) {
	start := time.Now()
	ctx = session.CtxWithSessionID(ctx, r.session.ID)
	reply, stats, err := r.agent.Process(ctx, r.session, in.Prompt, agent.WithStreamSink(in.Stream))
	if err != nil {
		return RunOutput{}, fmt.Errorf("agent: %w", err)
	}

	if r.session.Title == "" {
		titleClient := session.TitleChatFunc(func(ctx context.Context, msgs []model.Message) (string, model.Usage, error) {
			content, _, usage, err := r.llmClient.ChatCompletionBlocking(ctx, msgs, nil)
			return content, usage, err
		})
		title, titleUsage, titleErr := session.GenerateTitle(ctx, titleClient, r.session.Messages)
		if titleErr != nil {
			slog.Warn("LLM title generation failed, using fallback", "err", titleErr)
		} else {
			if titleUsage.PromptTokens > 0 || titleUsage.CompletionTokens > 0 {
				r.session.PromptTokens += titleUsage.PromptTokens
				r.session.CompletionTokens += titleUsage.CompletionTokens
			}
			if title != "" {
				r.session.Title = title
			}
		}
	}

	r.session.PromptTokens += stats.PromptTokens
	r.session.CompletionTokens += stats.CompletionTokens
	if err := session.PersistAfterReply(r.session, r.sessionsDir, r.workspace, 100); err != nil {
		slog.Error("persist session failed", "err", err)
		return RunOutput{}, fmt.Errorf("persist session: %w", err)
	}

	return RunOutput{
		Reply:                 reply,
		Duration:              time.Since(start),
		ToolCalls:             stats.ToolCalls,
		PromptTokens:          stats.PromptTokens,
		CompletionTokens:      stats.CompletionTokens,
		TotalPromptTokens:     r.session.PromptTokens,
		TotalCompletionTokens: r.session.CompletionTokens,
		SessionID:             r.session.ID,
		Workspace:             r.workspace,
	}, nil
}

// FormatDuration formats a duration in a compact human-readable way.
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}

func buildBaseTools(client *llm.Client, ws *util.Workspace, skillPaths []string) ([]model.Tool, model.ToolRegistry, error) {
	registry := model.NewToolRegistry()
	registry.AppendTools(
		tools.NewReadFile(ws),
		tools.NewWriteFile(ws),
		tools.NewBash(ws),
		tools.NewGlob(ws),
		tools.NewEditFile(ws),
		tools.NewGrep(ws),
	)
	webFetch, err := tools.NewWebFetch(client, 15*time.Minute)
	if err != nil {
		return nil, model.ToolRegistry{}, err
	}
	registry.AppendTools(webFetch)
	todoWrite, err := tools.NewTodoWrite()
	if err != nil {
		return nil, model.ToolRegistry{}, err
	}
	registry.AppendTools(todoWrite)
	skillTool, err := tools.NewSkill(skillPaths)
	if err != nil {
		return nil, model.ToolRegistry{}, err
	}
	registry.AppendTools(skillTool)
	return registry.Tools(), registry, nil
}

func buildAgentTypes(baseTools []model.Tool, registry model.ToolRegistry, workspaceDir string) map[string]tools.AgentTypeConfig {
	agentTypes := make(map[string]tools.AgentTypeConfig, len(tools.BuiltinAgentDefs))
	for _, def := range tools.BuiltinAgentDefs {
		resolved := resolveAgentTypeTools(def.Name, def.ToolNames, baseTools, registry, "built-in agent references unknown tool")
		agentTypes[def.Name] = tools.AgentTypeConfig{
			Tools:        resolved,
			SystemPrompt: def.SystemPrompt,
			Description:  def.Description,
		}
	}

	defs, err := tools.LoadAgentDefsFromPaths(config.AgentDefsSearchPaths(workspaceDir))
	if err != nil {
		slog.Warn("load agent defs failed", "err", err)
	}
	for _, def := range defs {
		if _, exists := agentTypes[def.Name]; exists {
			slog.Warn("skip user-defined agent: name conflicts with built-in", "name", def.Name)
			continue
		}
		resolved := resolveAgentTypeTools(def.Name, def.ToolNames, nil, registry, "skip unknown tool in agent def")
		if len(resolved) == 0 {
			slog.Warn("skip user-defined agent: no valid tools resolved", "name", def.Name)
			continue
		}
		agentTypes[def.Name] = tools.AgentTypeConfig{
			Tools:        resolved,
			SystemPrompt: def.SystemPrompt,
			Description:  def.Description,
		}
		slog.Info("loaded user-defined agent", "name", def.Name, "tools", len(resolved))
	}

	return agentTypes
}

func resolveAgentTypeTools(agentName string, toolNames []string, defaultTools []model.Tool, registry model.ToolRegistry, warnMsg string) []model.Tool {
	if toolNames == nil {
		return defaultTools
	}
	resolved := make([]model.Tool, 0, len(toolNames))
	for _, name := range toolNames {
		t := registry.Lookup(name)
		if t == nil {
			slog.Warn(warnMsg, "agent", agentName, "tool", name)
			continue
		}
		resolved = append(resolved, t)
	}
	return resolved
}

func loadOrCreateSession(sessionsDir, sessionID string) (*session.Session, error) {
	if sessionID == "" {
		return session.NewSession(""), nil
	}
	sess, err := session.LoadFromDir(sessionsDir, sessionID)
	if err == nil {
		slog.Info("resumed session", "id", sess.ID)
		return sess, nil
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		slog.Error("load session failed", "err", err)
		return nil, fmt.Errorf("load session: %w", err)
	}
	sess = session.NewSessionFromData(sessionID, "", time.Now(), nil, 0, 0)
	slog.Info("created session with id", "id", sess.ID)
	return sess, nil
}
