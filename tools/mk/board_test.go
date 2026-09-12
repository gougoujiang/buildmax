package main

import (
	"reflect"
	"testing"
)

func TestBoardBucketsClassifyByFrontmatter(t *testing.T) {
	tasks := []boardTask{
		{file: "10-done-dep.md"},                                      // stands in as a merged dep: absent from the queue below
		{file: "20-writing.md", claim: "alice 2026-09-13"},            // claimed, no PR -> in progress
		{file: "30-review.md", claim: "bob 2026-09-13", pr: "123"},    // PR set -> in review, even while claimed
		{file: "40-ready.md", dependsOn: []string{"99-merged.md"}},    // dep already merged (absent) -> ready
		{file: "50-blocked.md", dependsOn: []string{"20-writing.md"}}, // dep still in queue -> blocked
	}

	inProgress, inReview, ready, blocked := boardBuckets(tasks)

	assertFiles(t, "in progress", inProgress, []string{"20-writing.md"})
	assertFiles(t, "in review", inReview, []string{"30-review.md"})
	assertFiles(t, "ready", ready, []string{"10-done-dep.md", "40-ready.md"})
	assertFiles(t, "blocked", blocked, []string{"50-blocked.md"})
}

func assertFiles(t *testing.T, bucket string, got []boardTask, want []string) {
	t.Helper()
	var names []string
	for _, task := range got {
		names = append(names, task.file)
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("%s bucket = %v, want %v", bucket, names, want)
	}
}

func TestBoardParseFrontmatter(t *testing.T) {
	doc := `---
id: sample-task
title: Do the thing
roadmap: R1
depends_on: [20-other.md]
verification:
  - ./make test
  - ./make check docs
claim: alice 2026-09-13
pr: #123
---

## Outcome
Body text, not frontmatter.
`
	fields := parseBoardFrontmatter(doc)

	if got := fields["title"]; got != "Do the thing" {
		t.Errorf("title = %q", got)
	}
	if got := fields["roadmap"]; got != "R1" {
		t.Errorf("roadmap = %q", got)
	}
	if got := fields["claim"]; got != "alice 2026-09-13" {
		t.Errorf("claim = %q", got)
	}
	// A block list folds to the same comma-joined shape as an inline list.
	if got := boardList(fields["verification"]); !reflect.DeepEqual(got, []string{"./make test", "./make check docs"}) {
		t.Errorf("verification = %v", got)
	}
	if got := boardList(fields["depends_on"]); !reflect.DeepEqual(got, []string{"20-other.md"}) {
		t.Errorf("depends_on = %v", got)
	}
}

func TestBoardFirstListItemJoinsWrappedLines(t *testing.T) {
	body := "- A change that wraps across\n  two lines in the source.\n"
	if got := firstListItem(body); got != "A change that wraps across two lines in the source." {
		t.Errorf("firstListItem = %q", got)
	}
}
