// Package cli: root and subcommands for the BuildMax CLI.
package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/session"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var rootLong = fmt.Sprintf(`BuildMax – AI Agent CLI

  buildmax                    Start the TUI (new session)
  buildmax -r ID              Start the TUI with session ID
  buildmax -c                 Resume the most recent session (TUI or print mode)
  buildmax -p QUERY           Send QUERY to the LLM and print the response (no TUI)
  buildmax -r ID -p QUERY     Resume session ID, send QUERY, then print and save

Sessions:
  Each run with -p saves the session under the app data directory (see BUILDMAX_HOME or ~/.buildmax).
  Use -r/--resume <session-id> to continue a previous session (TUI or print mode).
  Use -c/--continue to resume the most recent session (by creation time); -r takes precedence if both are set.
  Use --session-id <uuid> to use a specific session ID (load if exists, else create); value must be a valid UUID.

Configuration:
  Run "buildmax init" to create a starter settings.yaml.
  Models are configured in BUILDMAX_HOME/settings.yaml (default ~/.buildmax).
  The first entry under models: is the default; select another with --model.
  Default model when none is configured: %s
`, config.DefaultModel)

// NewRootCommand creates and returns the root cobra command for BuildMax.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "buildmax",
		Short:         "BuildMax – AI Agent CLI",
		Long:          rootLong,
		RunE:          runRoot,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.Flags().BoolP("help", "h", false, "help for buildmax")
	root.Flags().StringP("print", "p", "", "send QUERY to the LLM and print the response (no TUI)")
	root.Flags().StringP("resume", "r", "", "session id to resume (TUI or print mode)")
	root.Flags().BoolP("continue", "c", false, "resume the most recent session (by creation time)")
	root.Flags().String("session-id", "", "use a specific session ID (load if exists, else create); must be a valid UUID")
	root.Flags().String("model", "", "use model from settings by model id or name")
	root.Flags().String("workspace", "", "workspace directory for the agent (default: current directory)")
	root.Flags().String("output", "text", "output format for -p print mode: text, json, jsonl")
	root.Flags().Bool("no-stream", false, "disable streaming of assistant reply to stdout in print mode")
	root.Flags().BoolP("quiet", "q", false, "suppress the stats footer in print text mode")
	root.Flags().Bool("include-deltas", false, "include llm_delta events in --output jsonl (verbose)")
	root.Flags().BoolP("version", "v", false, "print version and exit")
	root.AddCommand(newInitCommand())
	root.AddCommand(newVersionCommand())
	root.AddCommand(newLoginCommand())
	root.AddCommand(newLogoutCommand())
	root.AddCommand(newWhoamiCommand())
	root.AddCommand(newSandboxCommand())
	return root
}

func runRoot(cmd *cobra.Command, _ []string) error {
	if v, _ := cmd.Flags().GetBool("version"); v {
		fmt.Fprintf(os.Stdout, "buildmax version %s\n", config.VersionString())
		return nil
	}
	prompt, _ := cmd.Flags().GetString("print")
	resumeID, _ := cmd.Flags().GetString("resume")
	cont, _ := cmd.Flags().GetBool("continue")
	model, _ := cmd.Flags().GetString("model")
	sessionID, _ := cmd.Flags().GetString("session-id")
	workspace, _ := cmd.Flags().GetString("workspace")
	outputStr, _ := cmd.Flags().GetString("output")
	noStream, _ := cmd.Flags().GetBool("no-stream")
	quiet, _ := cmd.Flags().GetBool("quiet")
	includeDeltas, _ := cmd.Flags().GetBool("include-deltas")

	format, err := parseOutputFormat(outputStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return &ExitError{Code: ExitUsage, Err: err}
	}

	if sessionID != "" {
		if _, err := uuid.Parse(sessionID); err != nil {
			fmt.Fprintln(os.Stderr, "invalid session-id: not a valid UUID")
			return &ExitError{Code: ExitUsage, Err: fmt.Errorf("invalid session-id: %w", err)}
		}
	}

	var effectiveSessionID string
	if sessionID != "" {
		effectiveSessionID = sessionID
	} else {
		var err error
		effectiveSessionID, err = resolveResumeID(resumeID, cont)
		if err != nil {
			return err
		}
	}

	if err := checkModelConfig(); err != nil {
		return &ExitError{Code: ExitUsage, Err: err}
	}

	if prompt != "" {
		slog.Info("running print mode")
		return runPrintMode(printOptions{
			Prompt:        prompt,
			ResumeID:      effectiveSessionID,
			ModelName:     model,
			Workspace:     workspace,
			Format:        format,
			NoStream:      noStream,
			Quiet:         quiet,
			IncludeDeltas: includeDeltas,
		})
	}
	slog.Info("starting TUI")
	return runTUI(effectiveSessionID, model)
}

func parseOutputFormat(s string) (OutputFormat, error) {
	switch s {
	case "", "text":
		return OutputText, nil
	case "json":
		return OutputJSON, nil
	case "jsonl":
		return OutputJSONL, nil
	default:
		return OutputText, fmt.Errorf("invalid --output %q: want text, json, or jsonl", s)
	}
}

// checkModelConfig returns an error when the CLI has no usable model. It
// separates the three ways that happens — no file, a file with no models, and a
// file still holding the placeholder key `buildmax init` writes — because each
// one has a different next step, and "No model configured" answered none of
// them for a first-time user.
func checkModelConfig() error {
	path := config.SettingsPath()
	s, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if len(s.Models) == 0 {
		if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "No configuration found.\n\n"+
				"Run `buildmax init` to create %s, then add your API key.\n"+
				"Quickstart: %s\n", path, quickstartURL)
			return errors.New("no configuration file")
		}
		fmt.Fprintf(os.Stderr, "No model configured in %s.\n\n"+
			"Add a models: entry, or run `buildmax init --force` to regenerate the file.\n"+
			"Quickstart: %s\n", path, quickstartURL)
		return errors.New("no model configured")
	}
	if s.Models[0].APIKey == APIKeyPlaceholder {
		fmt.Fprintf(os.Stderr, "The first model in %s still has the placeholder API key.\n\n"+
			"Replace %s on the api_key line with a real key.\n", path, APIKeyPlaceholder)
		return errors.New("api key not set")
	}
	return nil
}

// resolveResumeID resolves the --continue flag: if cont is true and resumeID
// is empty, it loads the session list and returns the most recent session ID.
func resolveResumeID(resumeID string, cont bool) (string, error) {
	if !cont || resumeID != "" {
		return resumeID, nil
	}
	sessionsDir := config.SessionsDir()
	list, err := agentapp.LoadSessionList(sessionsDir)
	if err != nil {
		slog.Error("load session list failed", "err", err)
		return "", fmt.Errorf("load session list: %w", err)
	}
	last := latestSessionItem(list)
	if last == nil {
		fmt.Fprintln(os.Stderr, "no sessions found; create one with -p PROMPT or start the TUI")
		return "", fmt.Errorf("no sessions to continue")
	}
	slog.Info("continue with last session", "id", last.ID)
	return last.ID, nil
}

func latestSessionItem(entries []session.SessionItem) *session.SessionItem {
	var best *session.SessionItem
	var bestT time.Time
	for i := range entries {
		t, err := time.Parse(time.RFC3339, entries[i].CreatedAt)
		if err != nil {
			continue
		}
		if best == nil || t.After(bestT) {
			best = &entries[i]
			bestT = t
		}
	}
	return best
}
