package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"buildmax/internal/agent"
	"buildmax/internal/config"
	"buildmax/internal/llm"
	"buildmax/internal/session"
	"buildmax/internal/tools"
)

// setupResult holds everything returned by setupAgentAndSession.
type setupResult struct {
	Agent       *agent.Agent
	Session     *session.Session
	SessionsDir string
	CWD         string
	ModelName   string
}

// ---------------------------------------------------------------------------
// toolBuilder — accumulates tools, short-circuits on first error.
// ---------------------------------------------------------------------------

// toolBuilder collects agent.Tool instances and records the first error.
type toolBuilder struct {
	tools  []agent.Tool
	byName map[string]agent.Tool
	err    error
}

// newToolBuilder returns an initialised toolBuilder.
func newToolBuilder() *toolBuilder {
	return &toolBuilder{byName: make(map[string]agent.Tool)}
}

// add appends a tool if no prior error has occurred; records the first error.
func (b *toolBuilder) add(t agent.Tool, err error) {
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

// result returns the accumulated tools and lookup map, or the stored error.
func (b *toolBuilder) result() ([]agent.Tool, map[string]agent.Tool, error) {
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
func buildBaseTools(client *llm.Client, cwd string, skillPaths []string) ([]agent.Tool, map[string]agent.Tool, error) {
	b := newToolBuilder()
	b.add(tools.NewReadFile(cwd))
	b.add(tools.NewWriteFile(cwd))
	b.add(tools.NewWebFetch(client, 15*time.Minute))
	b.add(tools.NewTodoWrite())
	b.add(tools.NewBash(cwd))
	b.add(tools.NewGlob(cwd))
	b.add(tools.NewEditFile(cwd))
	b.add(tools.NewGrep(cwd))
	b.add(tools.NewSkill(skillPaths))
	return b.result()
}

// buildAgentTypes resolves built-in sub-agent types from tools.BuiltinAgentDefs
// and merges user-defined agent definitions.
func buildAgentTypes(baseTools []agent.Tool, toolsByName map[string]agent.Tool, cwd string) map[string]tools.AgentTypeConfig {
	agentTypes := make(map[string]tools.AgentTypeConfig, len(tools.BuiltinAgentDefs))
	for _, def := range tools.BuiltinAgentDefs {
		var resolved []agent.Tool
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
		var resolved []agent.Tool
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
func setupAgentAndSession(resumeID string) (setupResult, error) {
	cfg := config.LoadLLM()
	if cfg.APIKey == "" {
		return setupResult{}, fmt.Errorf("API key required. Set OPENROUTER_API_KEY or BUILDMAX_API_KEY")
	}
	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("get working directory", "err", err)
		return setupResult{}, fmt.Errorf("get working directory: %w", err)
	}
	client := llm.NewClient(cfg)

	baseTools, toolsByName, err := buildBaseTools(client, cwd, config.SkillSearchPaths(cwd))
	if err != nil {
		slog.Error("build base tools", "err", err)
		return setupResult{}, err
	}

	agentTypes := buildAgentTypes(baseTools, toolsByName, cwd)

	taskTool, err := tools.NewTask(client, agentTypes)
	if err != nil {
		slog.Error("create task tool", "err", err)
		return setupResult{}, fmt.Errorf("create task tool: %w", err)
	}

	a := agent.NewAgent(client, append(baseTools, taskTool))

	sessionsDir := config.SessionsDir()
	if err = os.MkdirAll(sessionsDir, 0755); err != nil {
		slog.Error("create sessions dir", "err", err)
		return setupResult{}, fmt.Errorf("create sessions dir: %w", err)
	}

	var sess *session.Session
	if resumeID != "" {
		sess, err = session.LoadFromDir(sessionsDir, resumeID)
		if err != nil {
			slog.Error("load session failed", "err", err)
			return setupResult{}, fmt.Errorf("load session: %w", err)
		}
		slog.Info("resumed session", "id", sess.ID())
	} else {
		sess = session.NewSession("")
	}

	return setupResult{
		Agent:       a,
		Session:     sess,
		SessionsDir: sessionsDir,
		CWD:         cwd,
		ModelName:   cfg.Model,
	}, nil
}
