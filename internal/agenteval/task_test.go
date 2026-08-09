package agenteval

import (
	"strings"
	"testing"
)

func TestParseTask_Valid(t *testing.T) {
	data := []byte(`---
id: test-task
title: My Test Task
timeout: 60
---

Do something useful in the workspace.

---grader---

test -f result.txt
`)
	task, err := ParseTask(data, "/tmp/fixture")
	if err != nil {
		t.Fatalf("ParseTask: %v", err)
	}
	if task.ID != "test-task" {
		t.Errorf("ID = %q, want test-task", task.ID)
	}
	if task.Title != "My Test Task" {
		t.Errorf("Title = %q, want My Test Task", task.Title)
	}
	if task.TimeoutSecs != 60 {
		t.Errorf("TimeoutSecs = %d, want 60", task.TimeoutSecs)
	}
	if task.FixtureDir != "/tmp/fixture" {
		t.Errorf("FixtureDir = %q, want /tmp/fixture", task.FixtureDir)
	}
	if !strings.Contains(task.Prompt, "Do something useful") {
		t.Errorf("Prompt = %q, missing expected content", task.Prompt)
	}
	if !strings.Contains(task.Grader, "test -f result.txt") {
		t.Errorf("Grader = %q, missing expected content", task.Grader)
	}
}

// TestParseTask_CRLF covers task files checked out with Windows line endings:
// the frontmatter and grader separators are LF-only, so parsing has to
// normalize first or every section comes back empty.
func TestParseTask_CRLF(t *testing.T) {
	unix := `---
id: test-task
title: My Test Task
---

Do something useful in the workspace.

---grader---

test -f result.txt
`
	data := []byte(strings.ReplaceAll(unix, "\n", "\r\n"))
	task, err := ParseTask(data, "/tmp/fixture")
	if err != nil {
		t.Fatalf("ParseTask: %v", err)
	}
	if task.ID != "test-task" {
		t.Errorf("ID = %q, want test-task", task.ID)
	}
	if task.Prompt != "Do something useful in the workspace." {
		t.Errorf("Prompt = %q, want the prompt section without CR", task.Prompt)
	}
	if task.Grader != "test -f result.txt" {
		t.Errorf("Grader = %q, want the grader section without CR", task.Grader)
	}
}

func TestParseTask_DefaultTimeout(t *testing.T) {
	data := []byte(`---
id: minimal
title: Minimal
---
Do the thing.
---grader---
true
`)
	task, err := ParseTask(data, "")
	if err != nil {
		t.Fatalf("ParseTask: %v", err)
	}
	if task.TimeoutSecs != defaultTimeoutSecs {
		t.Errorf("TimeoutSecs = %d, want %d (default)", task.TimeoutSecs, defaultTimeoutSecs)
	}
}

func TestParseTask_TitleFallsBackToID(t *testing.T) {
	data := []byte(`---
id: my-task
---
Prompt.
---grader---
true
`)
	task, err := ParseTask(data, "")
	if err != nil {
		t.Fatalf("ParseTask: %v", err)
	}
	if task.Title != "my-task" {
		t.Errorf("Title = %q, want %q (fallback to ID)", task.Title, "my-task")
	}
}

func TestParseTask_MissingID(t *testing.T) {
	data := []byte(`---
title: No ID
---
Prompt.
---grader---
true
`)
	_, err := ParseTask(data, "")
	if err == nil {
		t.Fatal("expected error for missing id")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("error = %q, want mention of id", err.Error())
	}
}

func TestParseTask_MissingFrontmatter(t *testing.T) {
	_, err := ParseTask([]byte("no frontmatter here"), "")
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseTask_MissingGrader(t *testing.T) {
	data := []byte(`---
id: no-grader
---
Prompt but no grader section.
`)
	_, err := ParseTask(data, "")
	if err == nil {
		t.Fatal("expected error for missing grader section")
	}
}

func TestParseTask_FixtureOverriddenByFrontmatter(t *testing.T) {
	data := []byte(`---
id: override
fixture: /custom/path
---
Prompt.
---grader---
true
`)
	task, err := ParseTask(data, "/default/path")
	if err != nil {
		t.Fatalf("ParseTask: %v", err)
	}
	if task.FixtureDir != "/custom/path" {
		t.Errorf("FixtureDir = %q, want /custom/path (from frontmatter)", task.FixtureDir)
	}
}
