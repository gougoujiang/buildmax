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
	tools "buildmax/internal/execution/agenttool"
	"buildmax/internal/execution/mcptool"
	llm "buildmax/internal/infra/llm"
	"buildmax/internal/infra/mcp"
	"buildmax/internal/session"
	"buildmax/internal/util"
)

// Runtime holds the reusable agent execution state for one workspace and session.
type Runtime struct {
	Agent       *agent.Agent
	LLMClient   *llm.Client
	Session     *session.Session
	SessionsDir string
	Workspace   string
	ModelName   string

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
	Reply            string
	Duration         time.Duration
	ToolCalls        int
	PromptTokens     int
	CompletionTokens int
	SessionID        string
	Workspace        string
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

	workspaceDir := in.WorkspaceDir
	if workspaceDir == "" {
		workspaceDir, err = os.Getwd()
		if err != nil {
			slog.Error("get working directory", "err", err)
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}

	ws, err := util.NewWorkspace(workspaceDir)
	if err != nil {
		slog.Error("create workspace", "err", err)
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	client := llm.NewClient(llm.Config{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model})

	baseTools, toolsByName, err := buildBaseTools(client, ws, config.SkillSearchPaths(workspaceDir))
	if err != nil {
		slog.Error("build base tools", "err", err)
		return nil, err
	}

	var mcpCleanup func()
	if in.EnableMCP {
		mcpCfg, mcpErr := config.LoadMCPConfigForWorkspace(workspaceDir)
		if mcpErr != nil {
			slog.Error("load mcp config", "err", mcpErr)
			return nil, mcpErr
		}
		if mcpCfg != nil && len(mcpCfg.MCPServers) > 0 {
			ctx := context.Background()
			reg, regErr := mcp.NewRegistry(ctx, toMCPConfig(mcpCfg), nil)
			if regErr != nil {
				slog.Error("mcp registry", "err", regErr)
				return nil, fmt.Errorf("mcp: %w", regErr)
			}
			mcpCleanup = func() { _ = reg.Close() }
			for _, t := range mcptool.GatewayTools(reg) {
				baseTools = append(baseTools, t)
				toolsByName[t.Name()] = t
			}
		}
	}

	agentTypes := buildAgentTypes(baseTools, toolsByName, workspaceDir)

	runner, err := NewDefaultSubAgentRunner(client)
	if err != nil {
		slog.Error("create sub-agent runner", "err", err)
		return nil, fmt.Errorf("create sub-agent runner: %w", err)
	}

	taskTool, err := tools.NewTask(runner, agentTypes)
	if err != nil {
		slog.Error("create task tool", "err", err)
		return nil, fmt.Errorf("create task tool: %w", err)
	}

	effectivePrompt := agent.DefaultSystemPrompt
	if extra, err := agent.ReadAgentsMd(workspaceDir); err != nil {
		slog.Warn("read AGENTS.md", "err", err)
	} else if extra != "" {
		effectivePrompt = effectivePrompt + "\n\n" + extra
	}

	a := agent.NewAgent(client, append(baseTools, taskTool), agent.SystemPrompt(effectivePrompt))

	sessionsDir := config.SessionsDir()
	if err = os.MkdirAll(sessionsDir, 0755); err != nil {
		slog.Error("create sessions dir", "err", err)
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}

	sess, err := loadOrCreateSession(sessionsDir, in.SessionID)
	if err != nil {
		return nil, err
	}

	return &Runtime{
		Agent:       a,
		LLMClient:   client,
		Session:     sess,
		SessionsDir: sessionsDir,
		Workspace:   workspaceDir,
		ModelName:   modelName,
		mcpCleanup:  mcpCleanup,
	}, nil
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
	ctx = session.CtxWithSessionID(ctx, r.Session.ID())
	reply, stats, err := r.Agent.Process(ctx, r.Session, in.Prompt, agent.WithStreamSink(in.Stream))
	if err != nil {
		return RunOutput{}, fmt.Errorf("agent: %w", err)
	}

	if r.Session.Title() == "" {
		titleClient := session.TitleChatFunc(func(ctx context.Context, msgs []model.Message) (string, model.Usage, error) {
			content, _, usage, err := r.LLMClient.ChatWithTools(ctx, msgs, nil)
			return content, usage, err
		})
		title, titleUsage, titleErr := session.GenerateTitle(ctx, titleClient, r.Session.Messages())
		if titleErr != nil {
			slog.Warn("LLM title generation failed, using fallback", "err", titleErr)
		} else {
			if titleUsage.PromptTokens > 0 || titleUsage.CompletionTokens > 0 {
				r.Session.AddUsage(titleUsage.PromptTokens, titleUsage.CompletionTokens)
			}
			if title != "" {
				r.Session.SetTitle(title)
			}
		}
	}

	r.Session.AddUsage(stats.PromptTokens, stats.CompletionTokens)
	if err := session.PersistAfterReply(r.Session, r.SessionsDir, r.Workspace, 100); err != nil {
		slog.Error("persist session failed", "err", err)
		return RunOutput{}, fmt.Errorf("persist session: %w", err)
	}

	return RunOutput{
		Reply:            reply,
		Duration:         time.Since(start),
		ToolCalls:        stats.ToolCalls,
		PromptTokens:     stats.PromptTokens,
		CompletionTokens: stats.CompletionTokens,
		SessionID:        r.Session.ID(),
		Workspace:        r.Workspace,
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

// toolBuilder collects tools and records the first construction error.
type toolBuilder struct {
	tools  []model.Tool
	byName map[string]model.Tool
	err    error
}

func newToolBuilder() *toolBuilder {
	return &toolBuilder{byName: make(map[string]model.Tool)}
}

func (b *toolBuilder) add(t model.Tool, err error) {
	if b.err != nil {
		return
	}
	if err != nil {
		b.err = err
		return
	}
	b.tools = append(b.tools, t)
	b.byName[t.Name()] = t
}

func (b *toolBuilder) addTool(t model.Tool) {
	if b.err != nil {
		return
	}
	b.tools = append(b.tools, t)
	b.byName[t.Name()] = t
}

func (b *toolBuilder) result() ([]model.Tool, map[string]model.Tool, error) {
	if b.err != nil {
		return nil, nil, b.err
	}
	return b.tools, b.byName, nil
}

func buildBaseTools(client *llm.Client, ws *util.Workspace, skillPaths []string) ([]model.Tool, map[string]model.Tool, error) {
	b := newToolBuilder()
	b.addTool(tools.NewReadFile(ws))
	b.addTool(tools.NewWriteFile(ws))
	b.add(tools.NewWebFetch(client, 15*time.Minute))
	b.add(tools.NewTodoWrite())
	b.addTool(tools.NewBash(ws))
	b.addTool(tools.NewGlob(ws))
	b.addTool(tools.NewEditFile(ws))
	b.addTool(tools.NewGrep(ws))
	b.add(tools.NewSkill(skillPaths))
	return b.result()
}

func buildAgentTypes(baseTools []model.Tool, toolsByName map[string]model.Tool, workspaceDir string) map[string]tools.AgentTypeConfig {
	agentTypes := make(map[string]tools.AgentTypeConfig, len(tools.BuiltinAgentDefs))
	for _, def := range tools.BuiltinAgentDefs {
		var resolved []model.Tool
		if def.ToolNames == nil {
			resolved = baseTools
		} else {
			for _, tn := range def.ToolNames {
				if t, ok := toolsByName[tn]; ok {
					resolved = append(resolved, t)
				} else {
					slog.Warn("built-in agent references unknown tool", "agent", def.Name, "tool", tn)
				}
			}
		}
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
		var resolved []model.Tool
		for _, tn := range def.ToolNames {
			t, ok := toolsByName[tn]
			if !ok {
				slog.Warn("skip unknown tool in agent def", "agent", def.Name, "tool", tn)
				continue
			}
			resolved = append(resolved, t)
		}
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

func loadOrCreateSession(sessionsDir, sessionID string) (*session.Session, error) {
	if sessionID == "" {
		return session.NewSession(""), nil
	}
	sess, err := session.LoadFromDir(sessionsDir, sessionID)
	if err == nil {
		slog.Info("resumed session", "id", sess.ID())
		return sess, nil
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		slog.Error("load session failed", "err", err)
		return nil, fmt.Errorf("load session: %w", err)
	}
	sess = session.NewSessionFromData(sessionID, "", time.Now(), nil, 0, 0)
	slog.Info("created session with id", "id", sess.ID())
	return sess, nil
}
