package mcp

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRegistryInMemoryListAndCall(t *testing.T) {
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

	inSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "greet",
		Description: "greeting",
		InputSchema: inSchema,
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in map[string]any) (*mcpsdk.CallToolResult, map[string]any, error) {
		name, _ := in["name"].(string)
		return nil, map[string]any{"message": "hello " + name}, nil
	})

	tools, err := listAllTools(ctx, cliSess)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "greet" {
		t.Fatalf("tools=%v", tools)
	}

	by := map[string]*mcpsdk.Tool{"greet": tools[0]}
	states := []*serverState{{
		id:          "mem",
		session:     cliSess,
		toolsByName: by,
	}}
	reg := &Registry{
		catalog: buildCatalog(states),
		servers: states,
	}
	defer reg.Close()

	if reg.Catalog() == "" {
		t.Fatal("expected non-empty catalog")
	}
	detail, err := reg.ToolSchemaDetail("mem", "greet")
	if err != nil || detail == "" {
		t.Fatalf("detail=%q err=%v", detail, err)
	}
	out, err := reg.CallMcp(ctx, "mem", "greet", map[string]any{"name": "world"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello world") {
		t.Fatalf("got %q", out)
	}
}
