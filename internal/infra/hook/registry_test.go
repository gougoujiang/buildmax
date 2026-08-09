package hook

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

// TestNewDriverRegistry_NoDeps asserts the baseline registry exposes
// command and http, but not mcp_tool or prompt.
func TestNewDriverRegistry_NoDeps(t *testing.T) {
	r := NewDriverRegistry(Deps{})
	if _, ok := r[config.HookTypeCommand]; !ok {
		t.Error("command driver missing")
	}
	if _, ok := r[config.HookTypeHTTP]; !ok {
		t.Error("http driver missing")
	}
	if _, ok := r[config.HookTypeMCP]; ok {
		t.Error("mcp_tool driver present without MCPCaller dep")
	}
	if _, ok := r[config.HookTypePrompt]; ok {
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
	for _, want := range []string{config.HookTypeCommand, config.HookTypeHTTP, config.HookTypeMCP, config.HookTypePrompt} {
		if _, ok := r[want]; !ok {
			t.Errorf("driver %q missing", want)
		}
	}
}
