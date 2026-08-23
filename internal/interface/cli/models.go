package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/infra/llm"
	"github.com/gougoujiang/buildmax/internal/interface/auth"
	"github.com/gougoujiang/buildmax/internal/interface/client"
)

func newModelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List configured models, the local models a daemon holds, and the team models a server offers",
		Long: "Lists the models in settings.yaml and where each one sends prompts.\n\n" +
			"With --team, also lists the aliases that team may use through a BuildMax\n" +
			"server, which are the values a transport: buildmax entry names.\n\n" +
			"With --local, also lists what a local Ollama daemon has pulled, including\n" +
			"whether each model can call tools, and prints an entry ready to paste.",
		RunE: runModels,
	}
	cmd.Flags().String("team", "", "list the managed model aliases available to this team")
	cmd.Flags().Bool("local", false, "list the models a local Ollama daemon holds")
	cmd.Flags().String("ollama-url", config.DefaultOllamaBaseURL, "where the local daemon listens, for --local")
	return cmd
}

func runModels(cmd *cobra.Command, _ []string) error {
	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	printConfiguredModels(settings)

	if local, _ := cmd.Flags().GetBool("local"); local {
		baseURL, _ := cmd.Flags().GetString("ollama-url")
		if err := printOllamaModels(cmd.Context(), cmd.OutOrStdout(), baseURL); err != nil {
			return err
		}
	}

	teamID, _ := cmd.Flags().GetString("team")
	if teamID == "" {
		return nil
	}
	return printTeamModels(cmd.Context(), teamID)
}

func printConfiguredModels(settings config.Settings) {
	if len(settings.Models) == 0 {
		fmt.Fprintln(os.Stdout, "No models configured. Run `buildmax init` to write a starter settings.yaml.")
		return
	}
	fmt.Fprintln(os.Stdout, "Models in settings.yaml:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  NAME\tMODEL\tTRANSPORT\tDESTINATION")
	for _, entry := range settings.Models {
		cfg := agentapp.ModelConfigFromEntry(entry)
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n",
			cfg.Name, cfg.ProviderModel, modelTransportLabel(cfg), modelDestination(cfg))
	}
	_ = w.Flush()
}

// modelTransportLabel names the connection mode. It is always shown, including
// for direct entries, so "where does this send my prompts" never depends on a
// field being absent.
func modelTransportLabel(cfg agentapp.ModelConfig) string {
	if cfg.IsManaged() {
		return config.TransportBuildMax
	}
	return config.TransportDirect
}

// modelDestination is where prompts actually go.
func modelDestination(cfg agentapp.ModelConfig) string {
	if cfg.IsManaged() {
		server := cfg.ServerURL
		if server == "" {
			server = "(no server_url)"
		}
		team := cfg.TeamID
		if team == "" {
			team = "(no team_id)"
		}
		return server + " team " + team
	}
	if cfg.BaseURL == "" {
		if cfg.Provider == config.LLMProviderOllama {
			return config.DefaultOllamaBaseURL
		}
		return config.DefaultOpenRouterBaseURL
	}
	return cfg.BaseURL
}

// ollamaListTimeout bounds the daemon calls behind --local. The listing asks
// the daemon about each model it holds, and a slow one must not hang the
// command.
const ollamaListTimeout = 10 * time.Second

// printOllamaModels lists what a local daemon holds.
//
// Capabilities are the column that matters: a model without tool calling
// cannot run the agent loop, and that is invisible in the model name. The
// paste-ready entry at the end mirrors what --team prints, so configuring a
// local model is the same gesture as configuring a team one.
func printOllamaModels(ctx context.Context, out io.Writer, baseURL string) error {
	ctx, cancel := context.WithTimeout(ctx, ollamaListTimeout)
	defer cancel()

	installed, err := llm.OllamaInventory(ctx, baseURL)
	if err != nil {
		return fmt.Errorf("list local models: %w", err)
	}
	fmt.Fprintf(out, "\nLocal models on %s:\n", baseURL)
	if len(installed) == 0 {
		fmt.Fprintln(out, "  none — pull one, for example `ollama pull qwen3:8b`")
		return nil
	}

	described := make([]llm.OllamaModel, 0, len(installed))
	for _, m := range installed {
		// /api/tags carries neither the context length nor, on older daemons,
		// the capabilities. Asking per model is what makes the columns true.
		if shown, err := llm.OllamaShow(ctx, baseURL, m.Model); err == nil {
			shown.SizeBytes = m.SizeBytes
			if shown.ParameterSize == "" {
				shown.ParameterSize = m.ParameterSize
			}
			described = append(described, shown)
			continue
		}
		described = append(described, m)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  MODEL\tSIZE\tPARAMS\tCONTEXT\tCAPABILITIES")
	for _, m := range described {
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n",
			m.Model, humanSize(m.SizeBytes), orDash(m.ParameterSize),
			orDash(contextLabel(m.ContextWindow)), orDash(strings.Join(m.Capabilities, ",")))
	}
	_ = w.Flush()

	usable := firstToolCallingModel(described)
	if usable.Model == "" {
		fmt.Fprintln(out, "\nNone of these can call tools, which the agent loop needs.")
		fmt.Fprintln(out, "Pull one that can, for example `ollama pull qwen3:8b`.")
		return nil
	}
	fmt.Fprintf(out, "\nTo use one, add to settings.yaml:\n\n"+
		"  - model: %s\n    name: %s (local)\n    provider: %s\n    api_url: %s\n    context_window: %d\n",
		usable.Model, usable.Model, config.LLMProviderOllama, baseURL, suggestedContextWindow(usable))
	return nil
}

// firstToolCallingModel picks what to show in the paste-ready entry. A model
// that cannot call tools would be a suggestion that does not work.
func firstToolCallingModel(models []llm.OllamaModel) llm.OllamaModel {
	for _, m := range models {
		if m.HasCapability(llm.OllamaCapabilityTools) {
			return m
		}
	}
	return llm.OllamaModel{}
}

// suggestedContextWindow keeps the daemon's answer as a ceiling: a model's full
// trained length can be more than the machine can allocate.
func suggestedContextWindow(m llm.OllamaModel) int {
	if m.ContextWindow <= 0 {
		return config.DefaultContextWindow
	}
	return min(m.ContextWindow, config.DefaultContextWindow)
}

func contextLabel(window int) string {
	if window <= 0 {
		return ""
	}
	return strconv.Itoa(window)
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func humanSize(bytes int64) string {
	if bytes <= 0 {
		return "-"
	}
	const unit = 1000
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "kMGT"[exp])
}

func printTeamModels(ctx context.Context, teamID string) error {
	info, err := auth.Info()
	if err != nil {
		return fmt.Errorf("load auth: %w", err)
	}
	if !info.LoggedIn {
		return fmt.Errorf("not logged in: run `buildmax login`")
	}
	token, err := auth.TokenForServer(info.ServerURL)
	if err != nil {
		return err
	}

	models, err := client.NewClient(info.ServerURL).ListTeamModels(ctx, token, teamID)
	if err != nil {
		return fmt.Errorf("list team models: %w", err)
	}

	fmt.Fprintf(os.Stdout, "\nManaged models for team %s on %s:\n", teamID, info.ServerURL)
	if len(models) == 0 {
		fmt.Fprintln(os.Stdout, "  none — this team has no managed models")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  NAME\tCAPABILITIES\tDEFAULT")
	for _, m := range models {
		def := ""
		if m.Default {
			def = "yes"
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\n", m.Name, strings.Join(m.Capabilities, ","), def)
	}
	_ = w.Flush()

	fmt.Fprintf(os.Stdout, "\nTo use one, add to settings.yaml:\n\n"+
		"  - name: %s\n    model: %s\n    transport: %s\n    server_url: %s\n    team_id: %s\n",
		models[0].Name, models[0].Name, config.TransportBuildMax, info.ServerURL, teamID)
	return nil
}
