package mcpservers

import (
	"context"
	"testing"

	"buildmax/internal/config"
)

func TestProbeMCPServers_stdioFailsFast(t *testing.T) {
	cfg := &config.MCPConfigRoot{
		MCPServers: map[string]config.MCPServerConfig{
			"bad": {Type: "stdio", Command: "/nonexistent/mcp-probe-binary-xxxxx", Args: []string{}},
		},
	}
	ctx := context.Background()
	rows := ProbeMCPServers(ctx, cfg, nil)
	if len(rows) != 1 {
		t.Fatalf("len(rows)=%d want 1", len(rows))
	}
	if rows[0].ID != "bad" {
		t.Fatalf("id=%q", rows[0].ID)
	}
	if rows[0].OK {
		t.Fatal("expected probe failure")
	}
	if rows[0].Err == nil {
		t.Fatal("expected Err set")
	}
}

func TestProbeMCPServers_nilCfg(t *testing.T) {
	if got := ProbeMCPServers(context.Background(), nil, nil); got != nil {
		t.Fatalf("got %#v want nil", got)
	}
}

func TestProbeMCPServers_emptyServers(t *testing.T) {
	cfg := &config.MCPConfigRoot{MCPServers: map[string]config.MCPServerConfig{}}
	if got := ProbeMCPServers(context.Background(), cfg, nil); got != nil {
		t.Fatalf("got %#v want nil", got)
	}
}
