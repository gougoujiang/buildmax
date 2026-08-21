package mcp

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"

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

// An MCP server that returns a picture used to have it JSON-encoded into the
// result text, which sent the model a base64 blob and called it a tool result.
// It now comes back as a part, with a line of text saying what it is — the text
// is what makes the image safe to drop for a model that cannot read one.
func TestImageContentBecomesAPartNotABlobOfText(t *testing.T) {
	result := formatCallToolResult(&mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: "here is the page"},
			&mcpsdk.ImageContent{MIMEType: "image/png", Data: []byte("not-really-a-png")},
		},
	})

	if len(result.Parts) != 1 {
		t.Fatalf("got %d parts, want the image", len(result.Parts))
	}
	part := result.Parts[0]
	if part.Type != llm.ContentPartImage || part.MediaType != "image/png" {
		t.Errorf("part = %+v, want a png image part", part)
	}
	if part.Data != base64.StdEncoding.EncodeToString([]byte("not-really-a-png")) {
		t.Errorf("part data = %q, want the image base64-encoded", part.Data)
	}
	if !strings.Contains(result.Text, "here is the page") {
		t.Error("the server's own text was dropped")
	}
	if !strings.Contains(result.Text, "image/png") || !strings.Contains(result.Text, "16 B") {
		t.Errorf("text %q should say what the image was", result.Text)
	}
	if strings.Contains(result.Text, part.Data) {
		t.Error("the image was also dumped into the text, which is what this replaced")
	}
}

// Anything else keeps the old behavior: a line of JSON, and no part, because
// there is nothing a protocol could do with it.
func TestUnknownContentStaysText(t *testing.T) {
	result := formatCallToolResult(&mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.AudioContent{MIMEType: "audio/wav", Data: []byte("wav")}},
	})
	if len(result.Parts) != 0 {
		t.Errorf("got %d parts, want none for content no protocol here accepts", len(result.Parts))
	}
	if result.Text == "" {
		t.Error("unknown content produced no text at all")
	}
}
