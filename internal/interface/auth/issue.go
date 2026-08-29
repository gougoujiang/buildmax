package auth

import (
	"context"
	"fmt"

	coreissue "github.com/gougoujiang/buildmax/internal/core/issue"
	"github.com/gougoujiang/buildmax/internal/interface/client"
	"github.com/gougoujiang/buildmax/internal/tool"
)

// IssueSession is one local session's link to one team Issue.
//
// It carries what the session must be able to say out loud as well as what it
// needs to call: which server and team the work came from, and which Issue.
// Work crossing that boundary should be visible before it crosses, not
// inferable afterwards from a tool call.
//
// It lasts one run. The durable form is the local Issue bridge's to design.
type IssueSession struct {
	ServerURL string
	TeamID    string
	TeamName  string
	Issue     coreissue.Issue
	Client    tool.IssueClient
}

// OpenIssueSession scopes this session to one Issue on the server it is signed
// in to.
//
// Unlike the artifact capability, a failure here is returned rather than
// swallowed. Artifacts are resolved for every session and their absence just
// means the session has no server; an Issue is resolved only because someone
// asked for one by name, and starting anyway against no Issue would be
// answering a different request than the one made.
func OpenIssueSession(ctx context.Context, issueID string) (*IssueSession, error) {
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
	team, issue, err := client.NewClient(info.ServerURL).FindIssue(ctx, token, issueID)
	if err != nil {
		return nil, err
	}
	return &IssueSession{
		ServerURL: info.ServerURL,
		TeamID:    team.ID,
		TeamName:  team.Name,
		Issue:     issue,
		Client:    client.NewIssueClient(info.ServerURL, team.ID, issue.ID, TokenForServer),
	}, nil
}

// ToolClient is the Issue capability to hand the runtime, or nil when there is
// no session. Nil registers no tools, which is what a run with no Issue gets.
func (s *IssueSession) ToolClient() tool.IssueClient {
	if s == nil {
		return nil
	}
	return s.Client
}
