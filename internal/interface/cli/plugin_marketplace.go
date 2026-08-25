package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/interface/pluginmgr"

	"github.com/spf13/cobra"
)

// The Marketplace commands. Staging, digest verification, and the rename that
// puts a release in place live in internal/interface/pluginmgr, which Desktop
// runs too; what is here is how the CLI asks and what it prints.

func newPluginPublishCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "publish <path>",
		Short: "Pack a plugin directory and publish it to the Marketplace",
		Long: "The version comes from the directory's own plugin.yaml. Publishing an\n" +
			"existing version is refused, so a correction means editing and committing\n" +
			"the manifest rather than replacing what somebody already downloaded.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginPublish(cmd.Context(), cmd.OutOrStdout(), args[0])
		},
	}
}

func newPluginInstallCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "install <name>",
		Short: "Download a plugin from the Marketplace and install it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInstall(cmd.Context(), cmd.OutOrStdout(), installOptions(cmd, args[0], false))
		},
	}
	addInstallFlags(c, "install")
	return c
}

func newPluginUpdateCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "update <name>",
		Short: "Replace an installed Marketplace plugin with a newer release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInstall(cmd.Context(), cmd.OutOrStdout(), installOptions(cmd, args[0], true))
		},
	}
	addInstallFlags(c, "update")
	return c
}

func addInstallFlags(c *cobra.Command, verb string) {
	c.Flags().String("version", "", verb+" this exact version instead of the newest suitable one")
	c.Flags().Bool("allow-yanked", false, verb+" a release that was withdrawn")
}

func installOptions(cmd *cobra.Command, name string, requireInstalled bool) pluginmgr.Options {
	version, _ := cmd.Flags().GetString("version")
	allowYanked, _ := cmd.Flags().GetBool("allow-yanked")
	return pluginmgr.Options{
		Name: name, Version: version, AllowYanked: allowYanked, RequireInstalled: requireInstalled,
	}
}

func newPluginUninstallCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Remove an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			path, err := pluginmgr.Uninstall(args[0], force)
			if errors.Is(err, pluginmgr.ErrIsCheckout) {
				// The mechanism says what is true; the flag name is this
				// surface's own word for overriding it.
				return fmt.Errorf("%w.\nRemove it yourself, or pass --force to delete it "+
					"and anything uncommitted in it", err)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s.\n", path)
			return nil
		},
	}
	c.Flags().Bool("force", false, "remove a Git checkout too, discarding anything uncommitted in it")
	return c
}

func runPluginPublish(ctx context.Context, w io.Writer, path string) error {
	// Validating here is a courtesy, not the check that matters: the server
	// inspects what it receives. Failing before an upload is just faster.
	pkg, err := pluginmgr.Inspect(path)
	if err != nil {
		return err
	}
	if errs := coreplugin.Errors(pkg.Findings); len(errs) > 0 {
		for _, f := range errs {
			fmt.Fprintf(w, "  %s\n", f.String())
		}
		return invalidPluginExit()
	}

	session, err := pluginmgr.Open()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Publishing %s %s to %s\n", pkg.Manifest.Name, pkg.Manifest.Version, session.ServerURL())
	release, err := session.Publish(ctx, path)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Published %s %s\n  digest: %s\n", release.PluginName, release.Version, release.Digest)
	if release.Source.Dirty {
		fmt.Fprintln(w, "  packed from a working tree with uncommitted changes, which the record says")
	}
	return nil
}

func runPluginInstall(ctx context.Context, w io.Writer, opts pluginmgr.Options) error {
	session, err := pluginmgr.Open()
	if err != nil {
		return err
	}
	plan, err := session.Resolve(ctx, opts)
	if err != nil {
		return err
	}
	if plan.AlreadyInstalled {
		fmt.Fprintf(w, "%s %s is already installed.\n", opts.Name, plan.Release.Version)
		return nil
	}

	// Shown before the replacement, which is the moment the decision is still
	// open.
	fmt.Fprintf(w, "Installing %s %s (%s)\n", opts.Name, plan.Release.Version, humanBytes(plan.Release.SizeBytes))
	writeReleaseSummary(w, plan.Release)

	if err := session.Install(ctx, opts, plan.Release); err != nil {
		return err
	}
	fmt.Fprintf(w, "Installed %s %s. A run already in flight keeps the plugins it started with.\n",
		opts.Name, plan.Release.Version)
	return nil
}

// writeReleaseSummary shows what a release contributes before it is installed.
func writeReleaseSummary(w io.Writer, release coreplugin.Release) {
	if release.Digest != "" {
		fmt.Fprintf(w, "  digest:      %s\n", release.Digest)
	}
	if release.PublishedBy != "" {
		fmt.Fprintf(w, "  published by %s\n", release.PublishedBy)
	}
	insp := release.Inspection
	if len(insp.Skills) > 0 {
		fmt.Fprintf(w, "  skills:      %v\n", insp.Skills)
	}
	if len(insp.Subagents) > 0 {
		names := make([]string, 0, len(insp.Subagents))
		for _, s := range insp.Subagents {
			names = append(names, s.Name)
		}
		fmt.Fprintf(w, "  subagents:   %v\n", names)
	}
	for _, s := range insp.MCP {
		fmt.Fprintf(w, "  mcp server:  %s (%s %s%s)\n", s.ID, s.Transport, s.Executable, s.Host)
	}
	for _, h := range insp.Hooks {
		fmt.Fprintf(w, "  hook:        %s %s %s%s\n", h.Event, h.Type, h.Executable, h.Host)
	}
	if len(insp.EnvRefs) > 0 {
		var missing []string
		for _, name := range insp.EnvRefs {
			if _, set := os.LookupEnv(name); !set {
				missing = append(missing, name)
			}
		}
		fmt.Fprintf(w, "  environment: %v\n", insp.EnvRefs)
		if len(missing) > 0 {
			fmt.Fprintf(w, "  NOT SET:     %v\n", missing)
		}
	}
	if release.Source.Dirty {
		fmt.Fprintln(w, "  packed from a working tree with uncommitted changes")
	}
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}
