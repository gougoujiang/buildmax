package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Fixture entities are modeled by their wire JSON, not the server's internal
// types: tools/mk cannot import internal/server, and only the shape crossing the
// API matters here.
type fxIssue struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Version uint64 `json:"version"`
}

type fxIssueList struct {
	Issues []fxIssue `json:"issues"`
}

type fxAgent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type fxWorkflow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type fxWorkflowList struct {
	Workflows []fxWorkflow `json:"workflows"`
}

type fixtureIssue struct {
	title       string
	description string
	status      string
	comments    []string
}

// kindFixtures seeds a small, deterministic, idempotent set of test data into a
// running kind cluster: a couple of accounts (each with its personal team), and
// for the first one an agent, a workflow that drives it, and issues spread
// across every status with a comment thread. It exists so automated Portal
// testing starts from populated list and detail views instead of the near-empty
// deployment `kind up` leaves behind. Rerunning changes nothing: every entity is
// matched by its fixture title or name and skipped when already present.
func kindFixtures() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	cluster := kindClusterName()
	exists, err := kindClusterExists(cluster)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("kind cluster %q does not exist; run %s kind up", cluster, mk())
	}

	ctx := context.Background()
	target := kindSmokeTarget()
	client := &http.Client{Timeout: 30 * time.Second}
	if err := waitForHTTP(ctx, client, target.apiBase+"/healthz", 60*time.Second); err != nil {
		return err
	}

	fmt.Printf("Seeding fixtures into %s (%s)\n", cluster, target.apiBase)

	// Accounts come first, through the operator CLI, because a personal team is
	// created with the user and every other fixture hangs off it. "already has
	// an account" is the idempotent success here, not a failure.
	for _, email := range []string{"alice@buildmax.local", "bob@buildmax.local"} {
		out, err := target.admin("user", "create", email)
		switch {
		case err == nil:
			fmt.Printf("  user %s: created\n", email)
		case strings.Contains(out, "already has an account"):
			fmt.Printf("  user %s: exists\n", email)
		default:
			return fmt.Errorf("create user %s: %w\n%s", email, err, out)
		}
	}

	// Alice carries the rich fixtures: an agent, a workflow that targets it, and
	// a spread of issues. Bob exists so a second account with its own personal
	// team and its own issues is present for boundary and list testing.
	if err := seedAliceFixtures(ctx, client, target); err != nil {
		return fmt.Errorf("seed alice@buildmax.local: %w", err)
	}
	if err := seedIssues(ctx, client, target, "bob@buildmax.local", []fixtureIssue{
		{title: "Triage inbound bug reports", description: "Weekly pass over new reports.", status: "in_progress"},
		{title: "Draft Q3 roadmap", description: "Collect themes from the team.", status: "todo"},
	}); err != nil {
		return fmt.Errorf("seed bob@buildmax.local: %w", err)
	}

	fmt.Printf("\nFixtures ready. Sign in with %s kind login <email> — try alice@buildmax.local.\n", mk())
	return nil
}

func seedAliceFixtures(ctx context.Context, client *http.Client, target smokeTarget) error {
	const email = "alice@buildmax.local"
	token, teamID, err := smokeSignIn(ctx, client, target, email)
	if err != nil {
		return err
	}

	agentID, err := ensureAgent(ctx, client, target, teamID, token, email,
		"Docs Writer",
		"Turns merged changes into release notes.",
		"You write concise, user-facing release notes from a list of changes.")
	if err != nil {
		return err
	}
	if err := ensureWorkflow(ctx, client, target, teamID, token, email,
		"Release Notes", "Draft release notes for a milestone.", agentID); err != nil {
		return err
	}

	return ensureIssues(ctx, client, target, teamID, token, email, []fixtureIssue{
		{title: "Set up CI pipeline", description: "Build, test, and lint on every PR.", status: "done"},
		{title: "Fix flaky login test", description: "Times out under load; suspect a race.", status: "in_progress",
			comments: []string{"Reproduced locally about one run in five.", "Narrowed it to the token refresh path."}},
		{title: "Write onboarding docs", description: "A new contributor should reach a green build in under an hour.", status: "todo"},
		{title: "Investigate memory leak", description: "Worker RSS climbs across long sessions.", status: "todo"},
	})
}

func seedIssues(ctx context.Context, client *http.Client, target smokeTarget, email string, specs []fixtureIssue) error {
	token, teamID, err := smokeSignIn(ctx, client, target, email)
	if err != nil {
		return err
	}
	return ensureIssues(ctx, client, target, teamID, token, email, specs)
}

func ensureIssues(ctx context.Context, client *http.Client, target smokeTarget, teamID, token, who string, specs []fixtureIssue) error {
	base := target.apiBase + "/api/teams/" + url.PathEscape(teamID) + "/issues"
	var existing fxIssueList
	if err := requestJSON(ctx, client, http.MethodGet, base, token, nil, &existing, http.StatusOK); err != nil {
		return err
	}
	byTitle := make(map[string]fxIssue, len(existing.Issues))
	for _, is := range existing.Issues {
		byTitle[is.Title] = is
	}

	for _, spec := range specs {
		issue, ok := byTitle[spec.title]
		if ok {
			fmt.Printf("  [%s] issue %q: exists\n", who, spec.title)
		} else {
			body := map[string]any{"title": spec.title, "description": spec.description}
			if err := requestJSON(ctx, client, http.MethodPost, base, token, body, &issue, http.StatusCreated); err != nil {
				return err
			}
			fmt.Printf("  [%s] issue %q: created\n", who, spec.title)
		}

		// A create always lands in "todo", so the status move is what puts an
		// issue in the other columns. It is a read-modify-write: the PATCH must
		// echo the version the issue currently carries.
		if spec.status != "" && spec.status != issue.Status {
			patch := map[string]any{"version": issue.Version, "status": spec.status}
			if err := requestJSON(ctx, client, http.MethodPatch, base+"/"+url.PathEscape(issue.ID), token, patch, &issue, http.StatusOK); err != nil {
				return err
			}
			fmt.Printf("    status -> %s\n", spec.status)
		}

		if len(spec.comments) > 0 {
			if err := ensureComments(ctx, client, target, teamID, token, issue.ID, spec.comments); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureComments adds the thread only when the issue has none. A comment carries
// no natural key to match on, so "already has any comment" is the idempotency
// signal — enough to keep reruns from stacking duplicate threads.
func ensureComments(ctx context.Context, client *http.Client, target smokeTarget, teamID, token, issueID string, bodies []string) error {
	base := target.apiBase + "/api/teams/" + url.PathEscape(teamID) + "/issues/" + url.PathEscape(issueID) + "/comments"
	var existing struct {
		Comments []struct {
			ID string `json:"id"`
		} `json:"comments"`
	}
	if err := requestJSON(ctx, client, http.MethodGet, base, token, nil, &existing, http.StatusOK); err != nil {
		return err
	}
	if len(existing.Comments) > 0 {
		return nil
	}
	for _, body := range bodies {
		var created struct {
			ID string `json:"id"`
		}
		if err := requestJSON(ctx, client, http.MethodPost, base, token, map[string]any{"body": body}, &created, http.StatusCreated); err != nil {
			return err
		}
	}
	fmt.Printf("    + %d comment(s)\n", len(bodies))
	return nil
}

func ensureAgent(ctx context.Context, client *http.Client, target smokeTarget, teamID, token, who, name, description, instructions string) (string, error) {
	base := target.apiBase + "/api/teams/" + url.PathEscape(teamID) + "/agents"
	var existing []fxAgent // the agent list is a bare JSON array
	if err := requestJSON(ctx, client, http.MethodGet, base, token, nil, &existing, http.StatusOK); err != nil {
		return "", err
	}
	for _, a := range existing {
		if a.Name == name {
			fmt.Printf("  [%s] agent %q: exists\n", who, name)
			return a.ID, nil
		}
	}
	var created fxAgent
	body := map[string]any{"name": name, "description": description, "instructions": instructions}
	if err := requestJSON(ctx, client, http.MethodPost, base, token, body, &created, http.StatusCreated); err != nil {
		return "", err
	}
	fmt.Printf("  [%s] agent %q: created\n", who, name)
	return created.ID, nil
}

func ensureWorkflow(ctx context.Context, client *http.Client, target smokeTarget, teamID, token, who, name, description, agentID string) error {
	base := target.apiBase + "/api/teams/" + url.PathEscape(teamID) + "/workflows"
	var existing fxWorkflowList
	if err := requestJSON(ctx, client, http.MethodGet, base, token, nil, &existing, http.StatusOK); err != nil {
		return err
	}
	for _, w := range existing.Workflows {
		if w.Name == name {
			fmt.Printf("  [%s] workflow %q: exists\n", who, name)
			return nil
		}
	}
	// One agent_task step is the minimum a definition will validate with, and it
	// must target a real agent in this team — hence the agent is seeded first.
	def := fmt.Sprintf(`{"steps":[{"step_id":"draft","type":"agent_task","target_agent_id":%q,"prompt":"Draft the release notes from the merged changes."}]}`, agentID)
	body := map[string]any{"name": name, "description": description, "definition": def}
	var created fxWorkflow
	if err := requestJSON(ctx, client, http.MethodPost, base, token, body, &created, http.StatusCreated); err != nil {
		return err
	}
	fmt.Printf("  [%s] workflow %q: created\n", who, name)
	return nil
}
