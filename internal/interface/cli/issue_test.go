package cli

import (
	"strings"
	"testing"

	coreissue "github.com/icloudbb/buildmax/internal/core/issue"
	"github.com/icloudbb/buildmax/internal/interface/auth"
)

// A session working a space issue says so before it does anything: which
// server, which space, which issue, and where prompts go. A person who did not
// mean to hand a space's issue to a personal model learns it here.
func TestIssueSessionNoticeNamesTheBoundary(t *testing.T) {
	session := &auth.IssueSession{
		ServerURL: "https://buildmax.example",
		SpaceID:   "tm_1",
		SpaceName: "Platform",
		Issue:     coreissue.Issue{ID: "i_1", Title: "Ship the importer", Status: coreissue.StatusTodo},
	}

	managed := issueSessionNotice(session, auth.ModelSource{ServerURL: "https://buildmax.example"})
	for _, want := range []string{"i_1", "Ship the importer", "Platform", "https://buildmax.example", "Prompts go to"} {
		if !strings.Contains(managed, want) {
			t.Fatalf("managed notice missing %q:\n%s", want, managed)
		}
	}

	local := issueSessionNotice(session, auth.ModelSource{})
	if !strings.Contains(local, "straight from this machine") {
		t.Fatalf("a local-model session is not told its prompts leave this machine:\n%s", local)
	}
	// The agent's limits are part of the disclosure: it can read and report,
	// and it cannot move the work.
	if !strings.Contains(local, "stay yours to change") {
		t.Fatalf("notice does not say what the agent cannot do:\n%s", local)
	}
}

// No issue, no notice. An ordinary local session says nothing about servers.
func TestIssueSessionNoticeIsEmptyWithoutAnIssue(t *testing.T) {
	if got := issueSessionNotice(nil, auth.ModelSource{}); got != "" {
		t.Fatalf("an ordinary session printed %q", got)
	}
}

// A local agent's report is not the same as a run the deployment scheduled, and
// the thread must not read as though it were.
func TestCommentAuthorLabelSeparatesLocalFromScheduled(t *testing.T) {
	scheduled := commentAuthorLabel(coreissue.Comment{AuthorKind: coreissue.CommentAuthorAgent, AuthorID: "a_1"})
	local := commentAuthorLabel(coreissue.Comment{AuthorKind: coreissue.CommentAuthorLocalAgent, AuthorID: "u_1"})
	if scheduled == local {
		t.Fatal("a local report reads the same as a scheduled run")
	}
	if !strings.Contains(local, "locally") || !strings.Contains(local, "u_1") {
		t.Fatalf("local label = %q, want it named as local and attributed", local)
	}
	if !strings.Contains(scheduled, "server") {
		t.Fatalf("scheduled label = %q", scheduled)
	}
}

func TestOneLineFlattensAndTruncates(t *testing.T) {
	if got := oneLine("a\nb"); got != "a b" {
		t.Fatalf("oneLine = %q", got)
	}
	long := strings.Repeat("x", 200)
	if got := oneLine(long); len([]rune(got)) != 72 || !strings.HasSuffix(got, "…") {
		t.Fatalf("oneLine kept %d runes", len([]rune(got)))
	}
}

// `issue start` launches a run, so it exposes the same run flags as `buildmax`
// itself and takes exactly one issue id. Pinning that keeps a future flag added
// to the root run from silently missing this surface.
func TestIssueStartSharesRunFlagsAndTakesOneArg(t *testing.T) {
	root := NewRootCommand()
	start, _, err := root.Find([]string{"issue", "start"})
	if err != nil {
		t.Fatalf("find issue start: %v", err)
	}
	if start.Name() != "start" {
		t.Fatalf("resolved %q, want the start command", start.CommandPath())
	}
	// A representative spread across the run surface: a print/session flag, the
	// model and workspace, the sandbox, and one of the niche flags — enough to
	// catch `issue start` drifting off the shared set.
	for _, name := range []string{"print", "resume", "model", "workspace", "sandbox", "max-iterations", "append-system-prompt"} {
		if start.Flags().Lookup(name) == nil {
			t.Errorf("issue start is missing the run flag --%s", name)
		}
	}
	if err := start.Args(start, nil); err == nil {
		t.Error("issue start accepted zero args, want exactly one issue id")
	}
	if err := start.Args(start, []string{"a", "b"}); err == nil {
		t.Error("issue start accepted two args, want exactly one issue id")
	}
}

func TestIsKnownIssueStatus(t *testing.T) {
	for _, ok := range []string{coreissue.StatusTodo, coreissue.StatusInProgress, coreissue.StatusDone} {
		if !isKnownIssueStatus(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "blocked", "DONE"} {
		if isKnownIssueStatus(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
