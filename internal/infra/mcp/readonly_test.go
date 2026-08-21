package mcp

import (
	"context"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestToolIsReadOnly proves the annotation survives the wire. The permission
// design rests on readOnlyHint being present in what tools/list returns; this
// asserts it rather than inferring it from the SDK's struct definition.
func TestToolIsReadOnly(t *testing.T) {
	ctx := context.Background()
	impl := &mcpsdk.Implementation{Name: "test-srv", Version: "1"}
	server := mcpsdk.NewServer(impl, nil)
	client := mcpsdk.NewClient(impl, nil)
	st, ct := mcpsdk.NewInMemoryTransports()
	srvSess, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srvSess.Close()
	cliSess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := func(_ context.Context, _ *mcpsdk.CallToolRequest, _ map[string]any) (*mcpsdk.CallToolResult, map[string]any, error) {
		return nil, map[string]any{}, nil
	}
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "search",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, handler)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "create_issue",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: false},
	}, handler)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "unannotated"}, handler)

	tools, err := listAllTools(ctx, cliSess)
	if err != nil {
		t.Fatal(err)
	}
	by := make(map[string]*mcpsdk.Tool, len(tools))
	for _, tl := range tools {
		by[tl.Name] = tl
	}
	reg := &Registry{servers: []*serverState{{id: "mem", session: cliSess, toolsByName: by}}}
	defer reg.Close()

	for _, tc := range []struct {
		server, tool string
		want         bool
	}{
		{"mem", "search", true},
		{"mem", "create_issue", false},
		{"mem", "unannotated", false}, // absent is indistinguishable from false
		{"mem", "nonexistent", false},
		{"other", "search", false},
		{"", "", false},
	} {
		if got := reg.ToolIsReadOnly(tc.server, tc.tool); got != tc.want {
			t.Errorf("ToolIsReadOnly(%q, %q) = %v, want %v", tc.server, tc.tool, got, tc.want)
		}
	}

	if reg.ToolIsReadOnly("mem", "search") == (*Registry)(nil).ToolIsReadOnly("mem", "search") {
		t.Error("a nil registry must not report read-only")
	}
}
