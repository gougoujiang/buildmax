// Package main: root and subcommands for the BuildMax CLI.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"buildmax/internal/agent"
	"buildmax/internal/app"
	"buildmax/internal/config"
	"buildmax/internal/llm"
	"buildmax/internal/session"
	"buildmax/internal/tools"
	"buildmax/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

const rootLong = `BuildMax – AI Agent CLI

  buildmax                    Start the TUI (new session)
  buildmax -r ID              Start the TUI with session ID
  buildmax -c                 Resume the most recent session (TUI or prompt mode)
  buildmax -p PROMPT          Send PROMPT to the LLM and print the response (no TUI)
  buildmax -r ID -p PROMPT    Resume session ID, send PROMPT, then save

Sessions:
  Each run with -p saves the session under the app data directory (see HOME_DIR or ~/.buildmax).
  Use -r/--resume <session-id> to continue a previous session (TUI or prompt mode).
  Use -c/--continue to resume the most recent session (by creation time); -r takes precedence if both are set.

Environment (for -p):
  OPENROUTER_API_KEY or BUILDMAX_API_KEY   API key (required for -p)
  BUILDMAX_BASE_URL                        Base URL (default: OpenRouter)
  BUILDMAX_MODEL                          Model (default: openai/gpt-3.5-turbo)
`

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "buildmax",
		Short: "BuildMax – AI Agent CLI",
		Long:  rootLong,
		RunE:  runRoot,
	}
	root.Flags().BoolP("help", "h", false, "help for buildmax")
	root.Flags().StringP("prompt", "p", "", "prompt to send to the LLM; prints response and exits")
	root.Flags().StringP("resume", "r", "", "session id to resume (TUI or prompt mode)")
	root.Flags().BoolP("continue", "c", false, "resume the most recent session (by creation time)")
	root.AddCommand(newVersionCommand())
	return root
}

func runRoot(cmd *cobra.Command, _ []string) error {
	prompt, _ := cmd.Flags().GetString("prompt")
	resumeID, _ := cmd.Flags().GetString("resume")
	cont, _ := cmd.Flags().GetBool("continue")
	if cont && resumeID == "" {
		sessionsDir := filepath.Join(config.DataDir(), "sessions")
		list, err := session.LoadList(sessionsDir)
		if err != nil {
			slog.Error("load session list failed", "err", err)
			return fmt.Errorf("load session list: %w", err)
		}
		last := session.LastByCreatedAt(list)
		if last == nil {
			fmt.Fprintln(os.Stderr, "no sessions found; create one with -p PROMPT or start the TUI")
			return fmt.Errorf("no sessions to continue")
		}
		resumeID = last.ID
		slog.Info("continue with last session", "id", resumeID)
	}
	if prompt != "" {
		slog.Info("running prompt mode")
		return runPromptMode(prompt, resumeID)
	}
	slog.Info("starting TUI")
	return runTUI(resumeID)
}

// setupResult holds everything returned by setupAgentAndSession.
type setupResult struct {
	Agent       *agent.Agent
	Session     *session.Session
	SessionsDir string
	CWD         string
	ModelName   string
}

// buildBaseTools constructs all base tools (Task is excluded — sub-agents must not recurse).
// Returns the tool slice and a name→tool lookup map.
func buildBaseTools(client *llm.Client, cwd string, skillPaths []string) ([]agent.Tool, map[string]agent.Tool, error) {
	readFileTool, err := tools.NewReadFile(cwd)
	if err != nil {
		return nil, nil, fmt.Errorf("create read_file tool: %w", err)
	}
	writeFileTool, err := tools.NewWriteFile(cwd)
	if err != nil {
		return nil, nil, fmt.Errorf("create write_file tool: %w", err)
	}
	webFetchTool, err := tools.NewWebFetch(client, 15*time.Minute)
	if err != nil {
		return nil, nil, fmt.Errorf("create webfetch tool: %w", err)
	}
	todoWriteTool, err := tools.NewTodoWrite()
	if err != nil {
		return nil, nil, fmt.Errorf("create todowrite tool: %w", err)
	}
	bashTool, err := tools.NewBash(cwd)
	if err != nil {
		return nil, nil, fmt.Errorf("create bash tool: %w", err)
	}
	globTool, err := tools.NewGlob(cwd)
	if err != nil {
		return nil, nil, fmt.Errorf("create glob tool: %w", err)
	}
	editFileTool, err := tools.NewEditFile(cwd)
	if err != nil {
		return nil, nil, fmt.Errorf("create edit_file tool: %w", err)
	}
	grepTool, err := tools.NewGrep(cwd)
	if err != nil {
		return nil, nil, fmt.Errorf("create grep tool: %w", err)
	}
	skillTool, err := tools.NewSkill(skillPaths)
	if err != nil {
		return nil, nil, fmt.Errorf("create skill tool: %w", err)
	}

	baseTools := []agent.Tool{readFileTool, writeFileTool, webFetchTool, todoWriteTool, bashTool, globTool, editFileTool, grepTool, skillTool}

	toolsByName := make(map[string]agent.Tool, len(baseTools))
	for _, t := range baseTools {
		toolsByName[t.Name()] = t
	}
	return baseTools, toolsByName, nil
}

// buildAgentTypes defines built-in sub-agent types and merges user-defined agent definitions.
func buildAgentTypes(baseTools []agent.Tool, toolsByName map[string]agent.Tool, cwd string) map[string]tools.AgentTypeConfig {
	// Resolve specific tools by name for built-in types.
	readFileTool := toolsByName["read_file"]
	globTool := toolsByName["glob"]
	grepTool := toolsByName["grep"]
	bashTool := toolsByName["bash"]

	agentTypes := map[string]tools.AgentTypeConfig{
		"general": {
			Tools:        baseTools,
			SystemPrompt: tools.GeneralSubAgentPrompt,
			Description:  "General-purpose agent with all tools for multi-step tasks.",
		},
		"explore": {
			Tools:        []agent.Tool{readFileTool, globTool, grepTool},
			SystemPrompt: tools.ExploreSubAgentPrompt,
			Description:  "Read-only agent for fast codebase exploration (Read, Glob, Grep).",
		},
		"shell": {
			Tools:        []agent.Tool{bashTool},
			SystemPrompt: tools.ShellSubAgentPrompt,
			Description:  "Command execution specialist (Bash only).",
		},
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

	sessionsDir := filepath.Join(config.DataDir(), "sessions")
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

// runTUI builds agent and session via setupAgentAndSession, then runs the TUI.
func runTUI(resumeID string) error {
	res, err := setupAgentAndSession(resumeID)
	if err != nil {
		return err
	}
	opts := tui.TUIOpts{
		Agent:       res.Agent,
		Session:     res.Session,
		ModelName:   res.ModelName,
		Workspace:   res.CWD,
		Version:     Version,
		SessionsDir: res.SessionsDir,
	}
	p := tea.NewProgram(app.NewModel(opts), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		slog.Error("TUI failed", "err", err)
		return fmt.Errorf("TUI: %w", err)
	}
	return nil
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Fprintf(os.Stdout, "buildmax version %s\n", Version)
		},
	}
}

func runPromptMode(prompt string, resumeID string) error {
	res, err := setupAgentAndSession(resumeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return err
	}
	ctx := context.Background()
	reply, err := res.Agent.Process(ctx, res.Session, prompt)
	slog.Debug("session details", "id", res.Session.ID(), "title", res.Session.Title(), "created_at", res.Session.CreatedAt())
	slog.Debug("session history", "messages", res.Session.Messages())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return fmt.Errorf("agent: %w", err)
	}
	if err := session.PersistAfterReply(res.Session, res.SessionsDir, res.CWD, 100); err != nil {
		slog.Error("persist session failed", "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return fmt.Errorf("persist session: %w", err)
	}
	slog.Info("agent reply", "len", len(reply))
	fmt.Println(reply)
	return nil
}
