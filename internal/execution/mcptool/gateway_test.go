package mcptool

import (
	"slices"
	"testing"
)

func TestGatewayToolNames(t *testing.T) {
	names := []string{ToolNameLoadMcpTools, ToolNameCallMcpTool}
	want := []string{"LoadMcpTools", "CallMcpTool"}
	if !slices.Equal(names, want) {
		t.Fatalf("%v", names)
	}
}
