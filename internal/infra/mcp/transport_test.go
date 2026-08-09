package mcp

import (
	"os"
	"strings"
	"testing"
)

func TestMergedEnv(t *testing.T) {
	base := os.Environ()
	merged := mergedEnv(map[string]string{"BUILDMAX_MCP_TEST_ONLY": "1"})
	if merged == nil {
		t.Fatal("nil")
	}
	var has bool
	for _, e := range merged {
		if strings.HasPrefix(e, "BUILDMAX_MCP_TEST_ONLY=1") {
			has = true
		}
	}
	if !has {
		t.Fatalf("missing override in %v", merged)
	}
	// Original keys should still be present (approximate length).
	if len(merged) < len(base) {
		t.Fatalf("merged shorter than base: %d vs %d", len(merged), len(base))
	}
}

func TestMergedEnv_nil(t *testing.T) {
	if mergedEnv(nil) != nil {
		t.Fatal("want nil")
	}
	if mergedEnv(map[string]string{}) != nil {
		t.Fatal("want nil")
	}
}
