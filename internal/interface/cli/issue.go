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
		Long: "Receive team work from the BuildMax server you are signed in to,\n" +
			"do it here, and say where it got to.\n\n" +
			"That is the whole scope: list what you were given, read one, work it\n" +
			"with `buildmax --issue`, and move its status when you are done. The\n" +
			"board, the workflow editor, and everything about who owns what stay in\n" +
			"Portal. Sign in with `buildmax login` first.",
	}
	cmd.AddCommand(newIssueListCommand())
	cmd.AddCommand(newIssueShowCommand())
	cmd.AddCommand(newIssueStatusCommand())
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

// issueSessionNotice says what a session working an Issue is about to do with
// team data, before it does any of it.
//
// The proposal's rule is that the server, the team, the Issue, and where the
// model sends prompts are visible before work crosses a boundary — not
// reconstructable afterwards from a tool call. A person who did not intend to
// hand a team's issue to a personal model should learn that here.
func issueSessionNotice(session *auth.IssueSession, source auth.ModelSource) string {
	if session == nil {
		return ""
	}
	team := session.TeamName
	if team == "" {
		team = session.TeamID
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Working issue %s — %s (%s) in team %s on %s.\n",
		session.Issue.ID, oneLine(session.Issue.Title), session.Issue.Status, team, session.ServerURL)
	b.WriteString("The agent can read that issue and post a report on it. Its status, assignee, and sub-issues stay yours to change.\n")
	if source.ServerURL != "" {
		fmt.Fprintf(&b, "Prompts go to %s.\n", source.ServerURL)
	} else {
		b.WriteString("Prompts go straight from this machine to whichever provider your model entry names; `buildmax models` shows which.\n")
	}
	return b.String()
}

func newIssueShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <issue-id>",
		Short: "Show one issue: what it asks for, how it was split up, and what has been said",
		Args:  cobra.ExactArgs(1),
		RunE:  runIssueShow,
	}
}

func newIssueStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status <issue-id> <todo|in_progress|done>",
		Short: "Move an issue's status",
		Long: "Moves an issue's status on the server.\n\n" +
			"This is a person's action on purpose. Status is what the team reads to\n" +
			"plan around, and `done` means someone accepted the work — so an agent\n" +
			"working the issue can say it believes the work is finished, and you\n" +
			"decide.\n\n" +
			"The change carries the version the issue was read at. If someone else\n" +
			"changed it in between, this refuses rather than overwriting them.",
		Args: cobra.ExactArgs(2),
		RunE: runIssueStatus,
	}
}

// issueSessionFor resolves the signed-in server and a token for it. Every issue
// command needs the same three things and fails the same three ways.
func issueSessionFor(cmd *cobra.Command) (serverURL, token string, err error) {
	info, err := auth.Info()
	if err != nil {
		return "", "", fmt.Errorf("read credentials: %w", err)
	}
	if !info.LoggedIn || info.ServerURL == "" {
		return "", "", fmt.Errorf("not signed in: run `buildmax login` first")
	}
	token, err = auth.TokenForServer(info.ServerURL)
	if err != nil {
		return "", "", fmt.Errorf("authenticate to %s: %w", info.ServerURL, err)
	}
	return info.ServerURL, token, nil
}

func runIssueShow(cmd *cobra.Command, args []string) error {
	serverURL, token, err := issueSessionFor(cmd)
	if err != nil {
		return err
	}
	c := client.NewClient(serverURL)
	team, issue, err := c.FindIssue(cmd.Context(), token, args[0])
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s  %s\n", issue.ID, issue.Title)
	fmt.Fprintf(out, "%s in team %s\n", issue.Status, team.Name)
	if issue.AssigneeKind != nil && issue.AssigneeID != nil {
		fmt.Fprintf(out, "assigned to %s %s\n", *issue.AssigneeKind, *issue.AssigneeID)
	}
	if strings.TrimSpace(issue.Description) != "" {
		fmt.Fprintf(out, "\n%s\n", strings.TrimSpace(issue.Description))
	}
	children, comments, omitted, err := c.IssueThread(cmd.Context(), token, team.ID, issue.ID)
	if err != nil {
		// The issue itself printed. Saying the rest could not be read beats
		// showing an issue that looks like it has no discussion.
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not read sub-issues or discussion: %v\n", err)
		return nil
	}
	if len(children) > 0 {
		fmt.Fprintln(out, "\nSub-issues:")
		for _, child := range children {
			fmt.Fprintf(out, "  %s  %-12s %s\n", child.ID, child.Status, oneLine(child.Title))
		}
	}
	if omitted > 0 {
		fmt.Fprintf(out, "\nDiscussion (%d older not shown):\n", omitted)
	} else if len(comments) > 0 {
		fmt.Fprintln(out, "\nDiscussion:")
	}
	for _, comment := range comments {
		fmt.Fprintf(out, "\n  %s — %s\n", commentAuthorLabel(comment), comment.CreatedAt.Local().Format("2006-01-02 15:04"))
		for _, line := range strings.Split(strings.TrimSpace(comment.Body), "\n") {
			fmt.Fprintf(out, "    %s\n", line)
		}
	}
	fmt.Fprintf(out, "\nWork on it here: buildmax --issue %s\n", issue.ID)
	return nil
}

// commentAuthorLabel names who is speaking, and keeps a report from a machine
// nobody scheduled distinct from a run this deployment recorded.
func commentAuthorLabel(comment coreissue.Comment) string {
	switch comment.AuthorKind {
	case coreissue.CommentAuthorAgent:
		return "agent (ran on the server)"
	case coreissue.CommentAuthorLocalAgent:
		return "agent (ran locally, reported by " + comment.AuthorID + ")"
	case coreissue.CommentAuthorSystem:
		return "BuildMax"
	default:
		return comment.AuthorID
	}
}

func runIssueStatus(cmd *cobra.Command, args []string) error {
	issueID, status := args[0], args[1]
	if !isKnownIssueStatus(status) {
		return fmt.Errorf("unknown status %q: use todo, in_progress, or done", status)
	}
	serverURL, token, err := issueSessionFor(cmd)
	if err != nil {
		return err
	}
	c := client.NewClient(serverURL)
	team, issue, err := c.FindIssue(cmd.Context(), token, issueID)
	if err != nil {
		return err
	}
	if issue.Status == status {
		fmt.Fprintf(cmd.OutOrStdout(), "%s is already %s.\n", issue.ID, status)
		return nil
	}
	updated, err := c.SetIssueStatus(cmd.Context(), token, team.ID, issue.ID, status, issue.Version)
	if err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n", updated.ID, issue.Status, updated.Status)
	return nil
}
