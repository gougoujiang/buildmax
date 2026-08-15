package cli

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"text/template"

	"github.com/gougoujiang/buildmax/internal/config"

	"github.com/spf13/cobra"
)

// APIKeyPlaceholder is what `buildmax init` writes when the user did not pass
// --api-key. checkModelConfig recognizes it, so a run that would otherwise fail
// inside the LLM client with a provider authentication error stops with an
// instruction instead.
const APIKeyPlaceholder = "REPLACE_WITH_YOUR_API_KEY"

// Defaults for the generated file. They match the quickstart in README.md so a
// user who follows either path ends up with the same configuration.
const (
	initDefaultModel         = "openai/gpt-4o-mini"
	initDefaultName          = "GPT-4o mini"
	initDefaultContextWindow = 128000
	openRouterKeysURL        = "https://openrouter.ai/keys"
	quickstartURL            = "https://github.com/gougoujiang/buildmax/blob/main/docs/start/quickstart.md"
)

//go:embed templates/settings.yaml.tmpl
var settingsTemplate string

// settingsValues fills templates/settings.yaml.tmpl.
type settingsValues struct {
	Model         string
	Name          string
	APIURL        string
	APIKey        string
	ContextWindow int
}

// renderSettings renders the starter settings file. Every string value goes
// through %q, so a key or model id containing YAML-significant characters
// still produces a file that parses back to what the user passed.
func renderSettings(v settingsValues) ([]byte, error) {
	tmpl, err := template.New("settings").
		Funcs(template.FuncMap{"q": func(s string) string { return strconv.Quote(s) }}).
		Parse(settingsTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse settings template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, v); err != nil {
		return nil, fmt.Errorf("render settings template: %w", err)
	}
	return buf.Bytes(), nil
}

func newInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a starter settings.yaml",
		Long: fmt.Sprintf(`Create a starter settings.yaml under BUILDMAX_HOME (default ~/.buildmax).

The generated file configures one model — %s through
OpenRouter unless you say otherwise. Pass --api-key to make it usable
immediately, or edit the api_key line afterwards.

  buildmax init                          # write the file, fill in the key later
  buildmax init --api-key sk-or-v1-...   # write a ready-to-use file
  buildmax init --model llama3.1 --api-url http://localhost:11434/v1

Any OpenAI-compatible endpoint works. An existing file is never overwritten
without --force.`, initDefaultModel),
		RunE:         runInit,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
	}
	cmd.Flags().String("api-key", "", "API key for the configured model (default: a placeholder to edit later)")
	cmd.Flags().String("model", initDefaultModel, "model id to configure as the default")
	cmd.Flags().String("api-url", config.DefaultOpenRouterBaseURL, "OpenAI-compatible base URL for the model")
	cmd.Flags().String("name", "", "display name for the model (default: the model id)")
	cmd.Flags().Int("context-window", 0, "context window in tokens (default: provider-appropriate)")
	cmd.Flags().Bool("force", false, "overwrite an existing settings.yaml")
	return cmd
}

func runInit(cmd *cobra.Command, _ []string) error {
	apiKey, _ := cmd.Flags().GetString("api-key")
	model, _ := cmd.Flags().GetString("model")
	apiURL, _ := cmd.Flags().GetString("api-url")
	name, _ := cmd.Flags().GetString("name")
	contextWindow, _ := cmd.Flags().GetInt("context-window")
	force, _ := cmd.Flags().GetBool("force")

	if model == "" {
		return &ExitError{Code: ExitUsage, Err: errors.New("--model cannot be empty")}
	}
	if apiURL == "" {
		return &ExitError{Code: ExitUsage, Err: errors.New("--api-url cannot be empty")}
	}
	values := settingsValues{
		Model:         model,
		Name:          name,
		APIURL:        apiURL,
		APIKey:        apiKey,
		ContextWindow: contextWindow,
	}
	if values.Name == "" {
		values.Name = model
		if model == initDefaultModel {
			values.Name = initDefaultName
		}
	}
	if values.APIKey == "" {
		values.APIKey = APIKeyPlaceholder
	}
	if values.ContextWindow == 0 {
		values.ContextWindow = config.DefaultContextWindow
		if model == initDefaultModel {
			values.ContextWindow = initDefaultContextWindow
		}
	}

	path := config.SettingsPath()
	if _, err := os.Stat(path); err == nil && !force {
		fmt.Fprintf(os.Stderr, "%s already exists.\n\nEdit it, or re-run with --force to replace it.\n", path)
		return &ExitError{Code: ExitUsage, Err: errors.New("settings.yaml already exists")}
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return &ExitError{Code: ExitGeneric, Err: fmt.Errorf("check %s: %w", path, err)}
	}

	body, err := renderSettings(values)
	if err != nil {
		return &ExitError{Code: ExitGeneric, Err: err}
	}
	// 0o700/0o600: the file holds an API key.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return &ExitError{Code: ExitGeneric, Err: fmt.Errorf("create %s: %w", filepath.Dir(path), err)}
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return &ExitError{Code: ExitGeneric, Err: fmt.Errorf("write %s: %w", path, err)}
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Wrote %s\n\n", path)
	if values.APIKey == APIKeyPlaceholder {
		fmt.Fprintf(out, "Next:\n")
		fmt.Fprintf(out, "  1. Replace %s on the api_key line with a real key.\n", APIKeyPlaceholder)
		if apiURL == config.DefaultOpenRouterBaseURL {
			fmt.Fprintf(out, "     OpenRouter keys: %s\n", openRouterKeysURL)
		}
		fmt.Fprintf(out, "  2. Run: buildmax -p \"Summarize what this project does\"\n\n")
	} else {
		fmt.Fprintf(out, "Ready. Try it in a directory you can revert:\n")
		fmt.Fprintf(out, "  buildmax -p \"Summarize what this project does\"\n\n")
	}
	fmt.Fprintf(out, "Quickstart: %s\n", quickstartURL)
	return nil
}
