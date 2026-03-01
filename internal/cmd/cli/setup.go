package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"buildmax/internal/agent"
	"buildmax/internal/config"
	"buildmax/internal/llm"
	"buildmax/internal/session"
	"buildmax/internal/core"
	"buildmax/internal/tools"
	"buildmax/internal/util"
)

// setupResult holds everything returned by setupAgentAndSession.
type setupResult struct {
	Agent       *agent.Agent
	LLMClient   *llm.Client
	Session     *session.Session
	SessionsDir string
	CWD         string
	ModelName   string
}

// ---------------------------------------------------------------------------
// toolBuilder — accumulates tools, short-circuits on first error.
// ---------------------------------------------------------------------------

// toolBuilder collects core.Tool instances and records the first error.
type toolBuilder struct {
	tools  []core.Tool
	byName map[string]core.Tool
	err    error
}

// newToolBuilder returns an initialised toolBuilder.
func newToolBuilder() *toolBuilder {
	return &toolBuilder{byName: make(map[string]core.Tool)}
}

// add appends a tool if no prior error has occurred; records the first error.
func (b *toolBuilder) add(t core.Tool, err error) {
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

// addTool appends a tool that cannot fail (e.g. workspace-based tools).
func (b *toolBuilder) addTool(t core.Tool) {
	if b.err != nil {
		return
	}
	b.tools = append(b.tools, t)
	b.byName[t.Name()] = t
}

// result returns the accumulated tools and lookup map, or the stored error.
func (b *toolBuilder) result() ([]core.Tool, map[string]core.Tool, error) {
	if b.err != nil {
		return nil, nil, b.err
	}
	return b.tools, b.byName, nil
}

// ---------------------------------------------------------------------------
// buildBaseTools / buildAgentTypes / setupAgentAndSession
// ---------------------------------------------------------------------------

// buildBaseTools constructs all base tools (Task is excluded — sub-agents must not recurse).
// Returns the tool slice and a name→tool lookup map.
func buildBaseTools(client *llm.Client, ws *util.Workspace, skillPaths []string) ([]core.Tool, map[string]core.Tool, error) {
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

// buildAgentTypes resolves built-in sub-agent types from tools.BuiltinAgentDefs
// and merges user-defined agent definitions.
func buildAgentTypes(baseTools []core.Tool, toolsByName map[string]core.Tool, cwd string) map[string]tools.AgentTypeConfig {
	agentTypes := make(map[string]tools.AgentTypeConfig, len(tools.BuiltinAgentDefs))
	for _, def := range tools.BuiltinAgentDefs {
		var resolved []core.Tool
		if def.ToolNames == nil {
			// nil means all base tools.
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

	// Load user-defined agent definitions from project and global directories.
	defs, err := tools.LoadAgentDefsFromPaths(config.AgentDefsSearchPaths(cwd))
	if err != nil {
		slog.Warn("load agent defs failed", "err", err)
	}
	for _, def := range defs {
		if _, exists := agentTypes[def.Name]; exists {
			slog.Warn("skip user-defined agent: name conflicts with built-in", "name", def.Name)
			continue
		}
		var resolved []core.Tool
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

// setupAgentAndSession loads config, builds tools and agent types, creates the agent,
// ensures the sessions directory exists, and loads or creates the session.
// sessionID: when non-empty, load from disk or create with this ID if not found; when empty, create a new session with a random ID.
// modelSelector selects a model from settings by id or name when non-empty; empty means default (first model or env fallback).
func setupAgentAndSession(sessionID string, modelSelector string) (setupResult, error) {
	cfg, modelName, err := config.EffectiveLLMWithSelector("", modelSelector)
	if err != nil {
		return setupResult{}, err
	}
	if cfg.APIKey == "" {
		return setupResult{}, fmt.Errorf("API key required. Set %s", config.EnvKeyBuildmaxAPIKey)
	}
	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("get working directory", "err", err)
		return setupResult{}, fmt.Errorf("get working directory: %w", err)
	}
	ws, err := util.NewWorkspace(cwd)
	if err != nil {
		slog.Error("create workspace", "err", err)
		return setupResult{}, fmt.Errorf("create workspace: %w", err)
	}
	client := llm.NewClient(cfg)

	baseTools, toolsByName, err := buildBaseTools(client, ws, config.SkillSearchPaths(cwd))
	if err != nil {
		slog.Error("build base tools", "err", err)
		return setupResult{}, err
	}

	agentTypes := buildAgentTypes(baseTools, toolsByName, cwd)

	runner, err := agent.NewDefaultSubAgentRunner(client)
	if err != nil {
		slog.Error("create sub-agent runner", "err", err)
		return setupResult{}, fmt.Errorf("create sub-agent runner: %w", err)
	}

	taskTool, err := tools.NewTask(runner, agentTypes)
	if err != nil {
		slog.Error("create task tool", "err", err)
		return setupResult{}, fmt.Errorf("create task tool: %w", err)
	}

	// Compose system prompt: default + optional AGENTS.md from workspace root (agents.md convention).
	effectivePrompt := agent.DefaultSystemPrompt
	if extra, err := agent.ReadAgentsMd(cwd); err != nil {
		slog.Warn("read AGENTS.md", "err", err)
	} else if extra != "" {
		effectivePrompt = effectivePrompt + "\n\n" + extra
	}

	// Main agent includes the task tool so it can delegate to sub-agents.
	a := agent.NewAgent(client, append(baseTools, taskTool), agent.SystemPrompt(effectivePrompt))

	sessionsDir := config.SessionsDir()
	if err = os.MkdirAll(sessionsDir, 0755); err != nil {
		slog.Error("create sessions dir", "err", err)
		return setupResult{}, fmt.Errorf("create sessions dir: %w", err)
	}

	var sess *session.Session
	if sessionID != "" {
		sess, err = session.LoadFromDir(sessionsDir, sessionID)
		if err != nil {
			if errors.Is(err, session.ErrSessionNotFound) {
				sess = session.NewSessionFromData(sessionID, "", time.Now(), nil, 0, 0)
				slog.Info("created session with id", "id", sess.ID())
			} else {
				slog.Error("load session failed", "err", err)
				return setupResult{}, fmt.Errorf("load session: %w", err)
			}
		} else {
			slog.Info("resumed session", "id", sess.ID())
		}
	} else {
		sess = session.NewSession("")
	}

	return setupResult{
		Agent:       a,
		LLMClient:   client,
		Session:     sess,
		SessionsDir: sessionsDir,
		CWD:         cwd,
		ModelName:   modelName,
	}, nil
}
