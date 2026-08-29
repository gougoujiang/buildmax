package client

import (
	"context"
	"fmt"
	"net/url"

	coreissue "github.com/gougoujiang/buildmax/internal/core/issue"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
)

// AssignedIssue is one item of the caller's inbox, with the team it belongs to
// kept alongside it.
//
// The team travels with the issue because a local surface has no current team:
// a login names a server and a person, and that person's work is spread across
// every team they are in. Anything the caller does next with this issue needs
// the team back.
type AssignedIssue struct {
	TeamID   string
	TeamName string
	Issue    coreissue.Issue
}

type teamsResponse []coreteam.Team

type issueListResponse struct {
	Issues []coreissue.Issue `json:"issues"`
	Total  int               `json:"total"`
}

// ListTeams returns the teams the caller belongs to.
func (c *Client) ListTeams(ctx context.Context, token string) ([]coreteam.Team, error) {
	var out teamsResponse
	if err := c.getJSON(ctx, token, "/api/teams", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListAssignedIssues returns what the caller has been assigned, across every
// team they belong to.
//
// One request per team, because the server has no cross-team listing and
// inventing one would put a route that reads every team a person is in behind a
// question only this inbox asks. A person's teams are few; if that stops being
// true, the fix is a route, not a wider fan-out here.
//
// A team that fails is skipped rather than failing the inbox: an inbox missing
// one team's work is more useful than no inbox, and the caller is told which
// team could not be read.
func (c *Client) ListAssignedIssues(ctx context.Context, token, status string, limit int) ([]AssignedIssue, []error) {
	teams, err := c.ListTeams(ctx, token)
	if err != nil {
		return nil, []error{fmt.Errorf("list teams: %w", err)}
	}
	var out []AssignedIssue
	var problems []error
	for _, team := range teams {
		issues, err := c.listAssignedInTeam(ctx, token, team.ID, status, limit)
		if err != nil {
			problems = append(problems, fmt.Errorf("team %s: %w", team.Name, err))
			continue
		}
		for _, issue := range issues {
			out = append(out, AssignedIssue{TeamID: team.ID, TeamName: team.Name, Issue: issue})
		}
	}
	return out, problems
}

func (c *Client) listAssignedInTeam(ctx context.Context, token, teamID, status string, limit int) ([]coreissue.Issue, error) {
	query := url.Values{}
	query.Set("assignee", "me")
	if status != "" {
		query.Set("status", status)
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprint(limit))
	}
	path := "/api/teams/" + url.PathEscape(teamID) + "/issues?" + query.Encode()
	var out issueListResponse
	if err := c.getJSON(ctx, token, path, &out); err != nil {
		return nil, err
	}
	return out.Issues, nil
}
