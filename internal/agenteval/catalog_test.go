package agenteval

import (
	"testing"
)

func TestLoadCatalog_NewStructure(t *testing.T) {
	tasks, err := LoadCatalog("../../eval") // internal/agenteval → internal → repo root → eval/
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{
		"001-read-summarize",
		"002-add-function",
		"003-rename-symbol",
		"004-fix-bug",
		"005-write-test",
		"006-implement-interface",
		"007-add-validation",
		"008-fix-multiple-bugs",
		"009-add-middleware",
		"010-implement-pagination",
		"011-implement-cache",
		"012-implement-retry",
		"013-worker-pool",
	}
	if len(tasks) != len(wantIDs) {
		t.Errorf("loaded %d tasks, want %d", len(tasks), len(wantIDs))
		for _, task := range tasks {
			t.Logf("  found: %s", task.ID)
		}
		return
	}
	for i, want := range wantIDs {
		if tasks[i].ID != want {
			t.Errorf("tasks[%d].ID = %q, want %q", i, tasks[i].ID, want)
		}
		if tasks[i].FixtureDir == "" {
			t.Errorf("tasks[%d].FixtureDir is empty", i)
		}
		if tasks[i].Prompt == "" {
			t.Errorf("tasks[%d].Prompt is empty", i)
		}
		if tasks[i].Grader == "" {
			t.Errorf("tasks[%d].Grader is empty", i)
		}
	}
}
