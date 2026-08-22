package hook

import (
	"testing"

	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
)

// TestNewDriverRegistry_NoDeps asserts the baseline registry exposes
// command and http, but not mcp_tool or prompt.
func TestNewDriverRegistry_NoDeps(t *testing.T) {
	r := NewDriverRegistry(Deps{})
	if _, ok := r[corehook.TypeCommand]; !ok {
		t.Error("command driver missing")
	}
	if _, ok := r[corehook.TypeHTTP]; !ok {
		t.Error("http driver missing")
	}
	if _, ok := r[corehook.TypeMCP]; ok {
		t.Error("mcp_tool driver present without MCPCaller dep")
	}
	if _, ok := r[corehook.TypePrompt]; ok {
		t.Error("prompt driver present without LLMCaller dep")
	}
}

// TestNewDriverRegistry_FullDeps asserts the full set when both callers are
// supplied.
func TestNewDriverRegistry_FullDeps(t *testing.T) {
	r := NewDriverRegistry(Deps{
		MCPCaller: &stubMCPCaller{},
		LLMCaller: &stubLLMCaller{},
	})
	for _, want := range []string{corehook.TypeCommand, corehook.TypeHTTP, corehook.TypeMCP, corehook.TypePrompt} {
		if _, ok := r[want]; !ok {
			t.Errorf("driver %q missing", want)
		}
	}
}
