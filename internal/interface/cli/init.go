package cli

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"text/template"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/infra/llm"

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
	// initDefaultOllamaModel is written when --ollama finds no daemon to ask.
	// A model that is not pulled yet is still the right thing to configure: the
	// next step printed is the command that pulls it.
	initDefaultOllamaModel = "qwen3:8b"
	// initOllamaProbeTimeout bounds the one call `init` makes to a daemon. The
	// file must be written whether or not anything answers.
	initOllamaProbeTimeout = 2 * time.Second
)

//go:embed templates/settings.yaml.tmpl
var settingsTemplate string

// settingsValues fills templates/settings.yaml.tmpl. An empty APIKey writes no
// api_key line at all, which is what a local provider needs: a placeholder
// there would be a secret that does not exist.
type settingsValues struct {
	Model         string
	Name          string
	Provider      string
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
  buildmax init --ollama                 # a local Ollama model, no key needed

With --ollama the entry points at a local daemon and carries no credential;
the model defaults to one the daemon already holds. Otherwise any
OpenAI-compatible endpoint works. An existing file is never overwritten
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
	cmd.Flags().Bool("ollama", false, "configure a local Ollama model instead of a hosted provider")
	return cmd
}

func runInit(cmd *cobra.Command, _ []string) error {
	apiKey, _ := cmd.Flags().GetString("api-key")
	model, _ := cmd.Flags().GetString("model")
	apiURL, _ := cmd.Flags().GetString("api-url")
	name, _ := cmd.Flags().GetString("name")
	contextWindow, _ := cmd.Flags().GetInt("context-window")
	force, _ := cmd.Flags().GetBool("force")
	ollama, _ := cmd.Flags().GetBool("ollama")

	provider := config.LLMProviderOpenAICompatible
	var local llm.OllamaModel
	if ollama {
		provider = config.LLMProviderOllama
		// Flags win over the daemon: --model names what the user wants
		// configured even if it is not pulled yet.
		local = resolveOllamaDefaults(cmd, apiURL, model, contextWindow)
		if !cmd.Flags().Changed("api-url") {
			apiURL = config.DefaultOllamaBaseURL
		}
		if !cmd.Flags().Changed("model") {
			model = local.Model
		}
		if contextWindow == 0 {
			contextWindow = local.ContextWindow
		}
		// A local model has no key, and the placeholder below must not apply.
		apiKey = ""
	}

	if model == "" {
		return &ExitError{Code: ExitUsage, Err: errors.New("--model cannot be empty")}
	}
	if apiURL == "" {
		return &ExitError{Code: ExitUsage, Err: errors.New("--api-url cannot be empty")}
	}
	values := settingsValues{
		Model:         model,
		Name:          name,
		Provider:      provider,
		APIURL:        apiURL,
		APIKey:        apiKey,
		ContextWindow: contextWindow,
	}
	if values.Name == "" {
		values.Name = model
		if model == initDefaultModel {
			values.Name = initDefaultName
		}
		if ollama {
			values.Name = model + " (local)"
		}
	}
	if values.APIKey == "" && config.LLMProviderNeedsAPIKey(provider) {
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
	if ollama {
		printOllamaNextSteps(out, values, local)
		fmt.Fprintf(out, "Quickstart: %s\n", quickstartURL)
		return nil
	}
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

// resolveOllamaDefaults asks the daemon what to configure. It never fails the
// command: a machine without a running daemon still gets a file, and the next
// steps printed say what to start and what to pull.
func resolveOllamaDefaults(cmd *cobra.Command, apiURL, model string, contextWindow int) llm.OllamaModel {
	baseURL := apiURL
	if !cmd.Flags().Changed("api-url") {
		baseURL = config.DefaultOllamaBaseURL
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), initOllamaProbeTimeout)
	defer cancel()

	chosen := model
	if !cmd.Flags().Changed("model") {
		chosen = initDefaultOllamaModel
		installed, err := llm.OllamaInventory(ctx, baseURL)
		if err != nil {
			return llm.OllamaModel{Model: chosen}
		}
		if len(installed) > 0 {
			chosen = installed[0].Model
		}
	}
	if contextWindow != 0 {
		return llm.OllamaModel{Model: chosen}
	}
	shown, err := llm.OllamaShow(ctx, baseURL, chosen)
	if err != nil {
		return llm.OllamaModel{Model: chosen}
	}
	shown.Model = chosen
	// The daemon reports what the model was trained for, which can be more than
	// the machine can allocate; the default is the ceiling, and the file is
	// where someone raises it.
	shown.ContextWindow = min(shown.ContextWindow, config.DefaultContextWindow)
	return shown
}

// printOllamaNextSteps says what is missing rather than what was written.
// Everything this provider can be wrong about — no daemon, no model, a model
// that cannot call tools — is fixed by one command.
func printOllamaNextSteps(out io.Writer, values settingsValues, local llm.OllamaModel) {
	fmt.Fprintf(out, "Next:\n")
	if len(local.Capabilities) == 0 {
		fmt.Fprintf(out, "  1. Start the daemon:  ollama serve\n")
		fmt.Fprintf(out, "  2. Pull the model:    ollama pull %s\n", values.Model)
		fmt.Fprintf(out, "  3. Check it:          buildmax doctor\n\n")
		return
	}
	if !local.HasCapability(llm.OllamaCapabilityTools) {
		fmt.Fprintf(out, "  1. %s does not support tool calling, so it cannot run the agent loop.\n", values.Model)
		fmt.Fprintf(out, "     Pick another with `buildmax models --local`, then edit %s.\n\n", config.SettingsPath())
		return
	}
	fmt.Fprintf(out, "Ready. Try it in a directory you can revert:\n")
	fmt.Fprintf(out, "  buildmax -p \"Summarize what this project does\"\n\n")
}
