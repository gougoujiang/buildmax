package cli

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/infra/pluginwire"
	"github.com/gougoujiang/buildmax/internal/interface/pluginmgr"

	"github.com/spf13/cobra"
)

// `plugin activations` is deliberately read-only. Activating is a decision with
// an audit trail and a capability report to read, and Portal is where both
// already live; what a terminal needs is the answer to "what did this run get,
// and at which version".

func newPluginActivationsCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "activations",
		Short: "List the plugins a team has activated for its background runs",
		Long: "Shows the exact release each activation is pinned to, whether it is\n" +
			"suspended, and whether the team curates this list or opens the whole\n" +
			"catalog. Changing an activation is done in Portal.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			teamID, _ := cmd.Flags().GetString("team")
			if teamID == "" {
				return fmt.Errorf("--team is required: an activation belongs to a team")
			}
			return runPluginActivations(cmd.Context(), cmd.OutOrStdout(), teamID)
		},
	}
	c.Flags().String("team", "", "the team whose activations to list")
	return c
}

func runPluginActivations(ctx context.Context, w io.Writer, teamID string) error {
	session, err := pluginmgr.Open()
	if err != nil {
		return err
	}
	got, err := session.ListActivations(ctx, teamID)
	if err != nil {
		return err
	}
	return writePluginActivations(w, *got)
}

func writePluginActivations(w io.Writer, got pluginwire.ActivationsResponse) error {
	fmt.Fprintf(w, "Curation: %s\n", curationDescription(got.Curation))
	if len(got.Activations) == 0 {
		fmt.Fprintln(w, "No plugins activated.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PLUGIN\tVERSION\tSTATE\tORIGIN\tACTIVATED BY")
	for _, a := range got.Activations {
		state := "active"
		if !a.Enabled {
			state = "suspended"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", a.PluginName, a.Version, state, a.Origin, a.ActivatedBy)
	}
	return tw.Flush()
}

// curationDescription spells out what the mode means rather than printing the
// bare word. "open" alone does not tell a reader whether an empty list means
// nothing is available or everything is.
func curationDescription(c coreplugin.Curation) string {
	if coreplugin.NormalizeCuration(string(c)) == coreplugin.CurationCurated {
		return "curated (an admin activates a plugin before an agent may name it)"
	}
	return "open (an agent may name any catalog plugin; naming activates it)"
}
