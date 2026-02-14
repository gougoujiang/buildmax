package tools

import (
	"testing"

	"buildmax/internal/util"
)

// testWorkspace creates a Workspace for test use. Fails the test on error.
func testWorkspace(t *testing.T, root string) *util.Workspace {
	t.Helper()
	ws, err := util.NewWorkspace(root)
	if err != nil {
		t.Fatalf("NewWorkspace(%q): %v", root, err)
	}
	return ws
}
