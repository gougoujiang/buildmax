package tool

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/util"
)

// testWorkspace resolves a workspace root string for test use. Fails the test on error.
func testWorkspace(t *testing.T, root string) string {
	t.Helper()
	resolved, err := util.ResolveWorkspaceRoot(root)
	if err != nil {
		t.Fatalf("ResolveWorkspaceRoot(%q): %v", root, err)
	}
	return resolved
}
