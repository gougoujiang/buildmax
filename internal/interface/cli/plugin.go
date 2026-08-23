package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/interface/pluginmgr"

	"github.com/spf13/cobra"
)

// newPluginCommand builds `buildmax plugin`. Phase A ships the local commands;
// publish, install, and update arrive with the Marketplace. See
// docs/design/plugin-marketplace.md §9.1.
func newPluginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Inspect and manage installed plugins",
		Long: "A plugin is a directory under <BUILDMAX_HOME>/plugins holding skills, subagents,\n" +
			"MCP servers, and hooks. Clone one there and it loads on the next run, or\n" +
			"install one your deployment has published.",
	}
	cmd.AddCommand(newPluginListCommand())
	cmd.AddCommand(newPluginStatusCommand())
	cmd.AddCommand(newPluginValidateCommand())
	cmd.AddCommand(newPluginEnableCommand())
	cmd.AddCommand(newPluginDisableCommand())
	cmd.AddCommand(newPluginPublishCommand())
	cmd.AddCommand(newPluginInstallCommand())
	cmd.AddCommand(newPluginUpdateCommand())
	cmd.AddCommand(newPluginUninstallCommand())
	cmd.AddCommand(newPluginActivationsCommand())
	return cmd
}

func newPluginListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins and where each came from",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writePluginList(cmd.OutOrStdout(), config.DiscoverPlugins())
		},
	}
}

func newPluginStatusCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "status [name]",
		Short: "Show what a plugin contributes, where it came from, and what shadowed it",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspace, _ := cmd.Flags().GetString("workspace")
			fetch, _ := cmd.Flags().GetBool("fetch")
			var name string
			if len(args) == 1 {
				name = args[0]
			}
			return writePluginStatus(cmd.Context(), cmd.OutOrStdout(), workspace, name, fetch)
		},
	}
	c.Flags().String("workspace", "", "workspace directory (default: current directory)")
	c.Flags().Bool("fetch", false,
		"contact each repository plugin's remote to report how far behind it is")
	return c
}

func newPluginValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [path]",
		Short: "Parse a plugin directory and report every problem in it",
		Long: "With no path, validates every installed plugin. Exits non-zero when anything\n" +
			"would stop a plugin from loading.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return writePluginValidatePath(cmd.OutOrStdout(), args[0])
			}
			return writePluginValidateAll(cmd.OutOrStdout(), config.DiscoverPlugins())
		},
	}
}

func newPluginEnableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Let a disabled plugin load again",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setPluginDisabled(cmd.OutOrStdout(), args[0], false)
		},
	}
}

func newPluginDisableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Stop a plugin from loading, without removing it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setPluginDisabled(cmd.OutOrStdout(), args[0], true)
		},
	}
}

// setPluginDisabled records the flag against the plugin's directory, which is
// what .state.json is keyed by.
func setPluginDisabled(w io.Writer, name string, disabled bool) error {
	resolved, err := pluginmgr.SetDisabled(name, disabled)
	if err != nil {
		return err
	}
	verb := "enabled"
	if disabled {
		verb = "disabled"
	}
	fmt.Fprintf(w, "%s %s. Runs already in flight keep the plugins they started with.\n", verb, resolved)
	return nil
}

func findPlugin(d config.PluginDiscovery, name string) (config.DiscoveredPlugin, bool) {
	for _, p := range d.Plugins {
		if p.Name() == name || p.Dir == name {
			return p, true
		}
	}
	return config.DiscoveredPlugin{}, false
}

func writePluginList(w io.Writer, d config.PluginDiscovery) error {
	if len(d.Plugins) == 0 {
		fmt.Fprintf(w, "No plugins installed. Clone one into %s to add it.\n", d.Dir)
		return writeDiscoveryNotes(w, d)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSOURCE\tSTATE\tDIRECTORY")
	for _, p := range d.Plugins {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Name(), pluginSourceLabel(p), pluginStateLabel(p), p.Path)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return writeDiscoveryNotes(w, d)
}

// writeDiscoveryNotes reports what the scan noticed about the directory itself.
func writeDiscoveryNotes(w io.Writer, d config.PluginDiscovery) error {
	if d.Policy.IsSet() {
		fmt.Fprintf(w, "\nOperator policy allows only these plugin sources: %v\n", d.Policy.AllowedSources)
	}
	if d.StateErr != nil {
		fmt.Fprintf(w, "\nPlugin state could not be read, so source and disabled state are unknown: %v\n", d.StateErr)
	}
	if len(d.Findings) > 0 {
		fmt.Fprintln(w)
		for _, f := range d.Findings {
			fmt.Fprintln(w, f.String())
		}
	}
	return nil
}

// pluginSourceLabel names where a directory came from, adding the release a
// Marketplace copy is.
func pluginSourceLabel(p config.DiscoveredPlugin) string {
	src := p.Source()
	if src == config.PluginSourceMarketplace && p.State.ReleaseVersion != "" {
		return "marketplace " + p.State.ReleaseVersion
	}
	return string(src)
}

func pluginStateLabel(p config.DiscoveredPlugin) string {
	switch {
	// A refusal is a decision, not a defect, and calling it an error would send
	// somebody looking for a problem with the plugin.
	case p.PolicyRefused:
		return "refused"
	case plugin.HasErrors(p.Findings):
		return "error"
	case p.State.Disabled:
		return "disabled"
	default:
		return "active"
	}
}
