package mcp

import (
	"context"
	"net/http"
	"slices"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServerProbeRow is one server line for diagnostics / TUI.
type MCPServerProbeRow struct {
	ID        string
	Type      string // stdio | sse | http
	OK        bool
	Err       error // nil when OK; Error() in UI when !OK
	ToolCount int   // meaningful when OK
}

// ProbeMCPServers connects to each server in cfg independently, runs tools/list,
// closes the session, and returns one row per server (sorted by ID).
// cfg must be non-nil with non-empty MCPServers.
func ProbeMCPServers(ctx context.Context, cfg *ConfigRoot, httpClient *http.Client) []MCPServerProbeRow {
	if cfg == nil || len(cfg.MCPServers) == 0 {
		return nil
	}
	ids := make([]string, 0, len(cfg.MCPServers))
	for id := range cfg.MCPServers {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	out := make([]MCPServerProbeRow, 0, len(ids))
	for _, id := range ids {
		entry := cfg.MCPServers[id]
		row := MCPServerProbeRow{ID: id, Type: entry.Type}
		n, err := probeOneServer(ctx, entry, httpClient)
		if err != nil {
			row.Err = err
		} else {
			row.OK = true
			row.ToolCount = n
		}
		out = append(out, row)
	}
	return out
}

func probeOneServer(ctx context.Context, entry ServerConfig, httpClient *http.Client) (toolCount int, err error) {
	transport, err := newTransport(entry, httpClient)
	if err != nil {
		return 0, err
	}
	impl := &mcpsdk.Implementation{Name: "buildmax", Version: "1"}
	client := mcpsdk.NewClient(impl, nil)
	sess, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = sess.Close() }()

	tools, err := listAllTools(ctx, sess)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range tools {
		if t != nil && t.Name != "" {
			n++
		}
	}
	return n, nil
}
