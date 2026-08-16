package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/interface/auth"
	"github.com/gougoujiang/buildmax/internal/interface/client"
)

func newModelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List configured models, and the team models a server offers",
		Long: "Lists the models in settings.yaml and where each one sends prompts.\n\n" +
			"With --team, also lists the aliases that team may use through a BuildMax\n" +
			"server, which are the values a transport: buildmax entry names.",
		RunE: runModels,
	}
	cmd.Flags().String("team", "", "list the managed model aliases available to this team")
	return cmd
}

func runModels(cmd *cobra.Command, _ []string) error {
	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	printLocalModels(settings)

	teamID, _ := cmd.Flags().GetString("team")
	if teamID == "" {
		return nil
	}
	return printTeamModels(cmd.Context(), teamID)
}

func printLocalModels(settings config.Settings) {
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
		return config.DefaultOpenRouterBaseURL
	}
	return cfg.BaseURL
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
	fmt.Fprintln(w, "  ALIAS\tNAME\tCAPABILITIES\tDEFAULT")
	for _, m := range models {
		def := ""
		if m.Default {
			def = "yes"
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", m.Alias, m.Name, strings.Join(m.Capabilities, ","), def)
	}
	_ = w.Flush()

	fmt.Fprintf(os.Stdout, "\nTo use one, add to settings.yaml:\n\n"+
		"  - name: Team %s\n    model: %s\n    transport: %s\n    server_url: %s\n    team_id: %s\n",
		models[0].Alias, models[0].Alias, config.TransportBuildMax, info.ServerURL, teamID)
	return nil
}
