package tool

import (
	"slices"
	"testing"
)

func TestGatewayToolNames(t *testing.T) {
	names := []string{ToolNameLoadMCPTools, ToolNameCallMCPTool}
	want := []string{"LoadMcpTools", "CallMcpTool"}
	if !slices.Equal(names, want) {
		t.Fatalf("%v", names)
	}
}
