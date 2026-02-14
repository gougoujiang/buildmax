// Package cmd: root and subcommands for the BuildMax CLI.
package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"buildmax/internal/config"
	"buildmax/internal/session"

	"github.com/spf13/cobra"
)

// Version is the application version, shown by the version subcommand.
var Version = "0.0.3"

const rootLong = `BuildMax – AI Agent CLI

  buildmax                    Start the TUI (new session)
  buildmax -r ID              Start the TUI with session ID
  buildmax -c                 Resume the most recent session (TUI or print mode)
  buildmax -p QUERY           Send QUERY to the LLM and print the response (no TUI)
  buildmax -r ID -p QUERY     Resume session ID, send QUERY, then print and save

Sessions:
  Each run with -p saves the session under the app data directory (see BUILDMAX_HOME or ~/.buildmax).
  Use -r/--resume <session-id> to continue a previous session (TUI or print mode).
  Use -c/--continue to resume the most recent session (by creation time); -r takes precedence if both are set.

Environment (for -p):
  OPENROUTER_API_KEY or BUILDMAX_API_KEY   API key (required for -p)
  BUILDMAX_BASE_URL                        Base URL (default: OpenRouter)
  BUILDMAX_MODEL                          Model (default: openai/gpt-3.5-turbo)
`

// NewRootCommand creates and returns the root cobra command for BuildMax.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "buildmax",
		Short: "BuildMax – AI Agent CLI",
		Long:  rootLong,
		RunE:  runRoot,
	}
	root.Flags().BoolP("help", "h", false, "help for buildmax")
	root.Flags().StringP("print", "p", "", "send QUERY to the LLM and print the response (no TUI)")
	root.Flags().StringP("resume", "r", "", "session id to resume (TUI or print mode)")
	root.Flags().BoolP("continue", "c", false, "resume the most recent session (by creation time)")
	root.Flags().String("model", "", "use model from settings by model id or name")
	root.Flags().BoolP("version", "v", false, "print version and exit")
	root.AddCommand(newVersionCommand())
	return root
}

func runRoot(cmd *cobra.Command, _ []string) error {
	if v, _ := cmd.Flags().GetBool("version"); v {
		fmt.Fprintf(os.Stdout, "buildmax version %s\n", Version)
		return nil
	}
	prompt, _ := cmd.Flags().GetString("print")
	resumeID, _ := cmd.Flags().GetString("resume")
	cont, _ := cmd.Flags().GetBool("continue")
	model, _ := cmd.Flags().GetString("model")

	resumeID, err := resolveResumeID(resumeID, cont)
	if err != nil {
		return err
	}

	if prompt != "" {
		slog.Info("running print mode")
		return runPrintMode(prompt, resumeID, model)
	}
	slog.Info("starting TUI")
	return runTUI(resumeID, model)
}

// resolveResumeID resolves the --continue flag: if cont is true and resumeID
// is empty, it loads the session list and returns the most recent session ID.
func resolveResumeID(resumeID string, cont bool) (string, error) {
	if !cont || resumeID != "" {
		return resumeID, nil
	}
	sessionsDir := config.SessionsDir()
	list, err := session.LoadList(sessionsDir)
	if err != nil {
		slog.Error("load session list failed", "err", err)
		return "", fmt.Errorf("load session list: %w", err)
	}
	last := session.LastByCreatedAt(list)
	if last == nil {
		fmt.Fprintln(os.Stderr, "no sessions found; create one with -p PROMPT or start the TUI")
		return "", fmt.Errorf("no sessions to continue")
	}
	slog.Info("continue with last session", "id", last.ID)
	return last.ID, nil
}
