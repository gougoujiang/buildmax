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
		runPromptMode(prompt, resumeID)
		return nil
	}
	slog.Info("starting TUI")
	if err := runTUI(resumeID); err != nil {
		return err
	}
	return nil
}

// runTUI builds agent, session (new or load), ensures sessions dir exists, and runs the TUI.
func runTUI(resumeID string) error {
	cfg := config.LoadLLM()
	if cfg.APIKey == "" {
		slog.Error("API key required for TUI")
		return fmt.Errorf("API key required. Set OPENROUTER_API_KEY or BUILDMAX_API_KEY")
	}
	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("get working directory", "err", err)
		return fmt.Errorf("get working directory: %w", err)
	}
	readFileTool, err := tools.NewReadFile(cwd)
	if err != nil {
		slog.Error("create read_file tool", "err", err)
		return fmt.Errorf("create read_file tool: %w", err)
	}
	client := llm.NewClient(cfg)
	a := agent.NewAgent(client, []agent.Tool{readFileTool})

	sessionsDir := filepath.Join(config.DataDir(), "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		slog.Error("create sessions dir", "err", err)
		return fmt.Errorf("create sessions dir: %w", err)
	}

	var sess *session.Session
	if resumeID != "" {
		sess, err = session.LoadFromDir(sessionsDir, resumeID)
		if err != nil {
			slog.Error("load session failed", "err", err)
			return fmt.Errorf("load session: %w", err)
		}
		slog.Info("resumed session", "id", sess.ID())
	} else {
		sess = session.NewSession("")
	}

	opts := tui.TUIOpts{
		Agent:       a,
		Session:     sess,
		ModelName:  cfg.Model,
		Workspace:  cwd,
		Version:    Version,
		SessionsDir: sessionsDir,
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

func runPromptMode(prompt string, resumeID string) {
	cfg := config.LoadLLM()
	if cfg.APIKey == "" {
		slog.Error("API key required")
		fmt.Fprintln(os.Stderr, "error: API key required. Set OPENROUTER_API_KEY or BUILDMAX_API_KEY.")
		os.Exit(1)
	}
	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("get working directory", "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	readFileTool, err := tools.NewReadFile(cwd)
	if err != nil {
		slog.Error("create read_file tool", "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	client := llm.NewClient(cfg)
	a := agent.NewAgent(client, []agent.Tool{readFileTool})
	ctx := context.Background()

	sessionsDir := filepath.Join(config.DataDir(), "sessions")
	var sess *session.Session
	if resumeID != "" {
		sess, err = session.LoadFromDir(sessionsDir, resumeID)
		if err != nil {
			slog.Error("load session failed", "err", err)
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		slog.Info("resumed session", "id", sess.ID())
	} else {
		sess = session.NewSession("")
	}

	reply, err := a.Process(ctx, sess, prompt)
	slog.Debug("session details", "id", sess.ID(), "title", sess.Title(), "created_at", sess.CreatedAt())
	slog.Debug("session history", "messages", sess.Messages())
	if err != nil {
		slog.Error("agent failed", "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	session.EnsureTitleFromFirstUserMessage(sess, 100)
	if err := session.SaveToDir(sess, sessionsDir); err != nil {
		slog.Error("save session failed", "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	entry := session.ListEntry{
		ID:        sess.ID(),
		Title:     sess.Title(),
		Workspace: cwd,
		CreatedAt: sess.CreatedAt().Format(time.RFC3339),
	}
	if err := session.UpsertListEntry(sessionsDir, entry); err != nil {
		slog.Error("upsert session list failed", "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	slog.Info("agent reply", "len", len(reply))
	fmt.Println(reply)
}
