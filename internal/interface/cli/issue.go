package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	coreissue "github.com/gougoujiang/buildmax/internal/core/issue"
	"github.com/gougoujiang/buildmax/internal/interface/auth"
	"github.com/gougoujiang/buildmax/internal/interface/client"
)

func newIssueCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "See the team work assigned to you",
		Long: "Reads issues from the BuildMax server you are signed in to.\n\n" +
			"This is the receiving end of team work, not a place to manage it:\n" +
			"the board, the workflow editor, and everything about who owns what\n" +
			"stay in Portal. Sign in with `buildmax login` first.",
	}
	cmd.AddCommand(newIssueListCommand())
	return cmd
}

func newIssueListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the issues assigned to you, across every team you are in",
		RunE:  runIssueList,
	}
	cmd.Flags().String("status", "", "only issues with this status: todo, in_progress, or done")
	cmd.Flags().Int("limit", 50, "most issues to list per team")
	return cmd
}

func runIssueList(cmd *cobra.Command, _ []string) error {
	info, err := auth.Info()
	if err != nil {
		return fmt.Errorf("read credentials: %w", err)
	}
	if !info.LoggedIn || info.ServerURL == "" {
		return fmt.Errorf("not signed in: run `buildmax login` to see the work a team assigned you")
	}
	status, _ := cmd.Flags().GetString("status")
	if status != "" && !isKnownIssueStatus(status) {
		return fmt.Errorf("unknown status %q: use todo, in_progress, or done", status)
	}
	limit, _ := cmd.Flags().GetInt("limit")

	token, err := auth.TokenForServer(info.ServerURL)
	if err != nil {
		return fmt.Errorf("authenticate to %s: %w", info.ServerURL, err)
	}
	issues, problems := client.NewClient(info.ServerURL).ListAssignedIssues(cmd.Context(), token, status, limit)
	// Problems are printed before the list rather than swallowed: an inbox that
	// quietly omits a team is worse than one that says which team it could not
	// read.
	for _, problem := range problems {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", problem)
	}
	if len(issues) == 0 {
		if len(problems) > 0 {
			return fmt.Errorf("no issues could be read")
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing is assigned to you.")
		return nil
	}
	printAssignedIssues(cmd, issues)
	return nil
}

func printAssignedIssues(cmd *cobra.Command, issues []client.AssignedIssue) {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ISSUE\tSTATUS\tTEAM\tTITLE")
	for _, item := range issues {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", item.Issue.ID, item.Issue.Status, item.TeamName, oneLine(item.Issue.Title))
	}
	_ = w.Flush()
}

// oneLine keeps a multi-line title from breaking the table. Truncation is the
// table's job; the whole title is in Portal.
func oneLine(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", " "), "\n", " "))
	const width = 72
	if len([]rune(s)) <= width {
		return s
	}
	return string([]rune(s)[:width-1]) + "…"
}

func isKnownIssueStatus(status string) bool {
	switch status {
	case coreissue.StatusTodo, coreissue.StatusInProgress, coreissue.StatusDone:
		return true
	}
	return false
}
