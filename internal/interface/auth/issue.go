package auth

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/interface/client"
	"github.com/gougoujiang/buildmax/internal/tool"
)

// IssueClientForSession scopes this session to one Issue on the server it is
// signed in to.
//
// Unlike the artifact capability, a failure here is returned rather than
// swallowed. Artifacts are resolved for every session and their absence just
// means the session has no server; an Issue is resolved only because someone
// asked for one by name, and starting anyway against no Issue would be
// answering a different request than the one made.
func IssueClientForSession(ctx context.Context, issueID string) (tool.IssueClient, error) {
	info, err := Info()
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	if !info.LoggedIn || info.ServerURL == "" {
		return nil, fmt.Errorf("not signed in: run `buildmax login` to work on a team issue")
	}
	token, err := TokenForServer(info.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("authenticate to %s: %w", info.ServerURL, err)
	}
	teamID, err := client.NewClient(info.ServerURL).FindIssueTeam(ctx, token, issueID)
	if err != nil {
		return nil, err
	}
	return client.NewIssueClient(info.ServerURL, teamID, issueID, TokenForServer), nil
}
