// Command mcp-echo-server is a minimal stdio MCP server for local testing of BuildMax MCP integration.
// Run from the repo root: go run ./cmd/mcp-echo-server
package main

import (
	"context"
	"log"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "mcp-echo-server", Version: "0.0.1"}, nil)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "echo",
		Description: "Echoes back the message string (demo tool for BuildMax MCP).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "Text to echo back",
				},
			},
			"required": []string{"message"},
		},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in map[string]any) (*mcpsdk.CallToolResult, map[string]any, error) {
		msg, _ := in["message"].(string)
		return nil, map[string]any{"echo": msg}, nil
	})
	if err := srv.Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
