package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Registry holds MCP client sessions and cached tool metadata per server.
type Registry struct {
	catalog string
	byID    map[string]*serverState
	order   []*serverState
}

type serverState struct {
	id           string
	transportTyp string
	session      *mcpsdk.ClientSession
	byTool       map[string]*mcpsdk.Tool
}

// NewRegistry connects every server in cfg, lists tools (with pagination), and builds the catalog string.
func NewRegistry(ctx context.Context, cfg *ConfigRoot, httpClient *http.Client) (*Registry, error) {
	if cfg == nil || len(cfg.MCPServers) == 0 {
		return nil, errors.New("mcpservers: empty config")
	}
	ids := make([]string, 0, len(cfg.MCPServers))
	for id := range cfg.MCPServers {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	var states []*serverState
	closeOnErr := func() {
		for i := len(states) - 1; i >= 0; i-- {
			_ = states[i].session.Close()
		}
	}

	impl := &mcpsdk.Implementation{Name: "buildmax", Version: "1"}
	for _, id := range ids {
		entry := cfg.MCPServers[id]
		transport, err := newTransport(entry, httpClient)
		if err != nil {
			closeOnErr()
			return nil, fmt.Errorf("mcp server %q: %w", id, err)
		}
		client := mcpsdk.NewClient(impl, nil)
		session, err := client.Connect(ctx, transport, nil)
		if err != nil {
			closeOnErr()
			return nil, fmt.Errorf("mcp server %q connect: %w", id, err)
		}
		tools, err := listAllTools(ctx, session)
		if err != nil {
			_ = session.Close()
			closeOnErr()
			return nil, fmt.Errorf("mcp server %q list tools: %w", id, err)
		}
		byName := make(map[string]*mcpsdk.Tool, len(tools))
		for _, t := range tools {
			if t == nil || t.Name == "" {
				continue
			}
			byName[t.Name] = t
		}
		states = append(states, &serverState{
			id:           id,
			transportTyp: entry.Type,
			session:      session,
			byTool:       byName,
		})
	}

	reg := &Registry{
		catalog: buildCatalog(states),
		byID:    make(map[string]*serverState, len(states)),
		order:   states,
	}
	for _, s := range states {
		reg.byID[s.id] = s
	}
	return reg, nil
}

func listAllTools(ctx context.Context, cs *mcpsdk.ClientSession) ([]*mcpsdk.Tool, error) {
	var out []*mcpsdk.Tool
	cursor := ""
	for {
		params := &mcpsdk.ListToolsParams{}
		if cursor != "" {
			params.Cursor = cursor
		}
		res, err := cs.ListTools(ctx, params)
		if err != nil {
			return nil, err
		}
		out = append(out, res.Tools...)
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	return out, nil
}

func buildCatalog(states []*serverState) string {
	var b strings.Builder
	for _, s := range states {
		fmt.Fprintf(&b, "Server %q (transport: %s):\n", s.id, s.transportTyp)
		if len(s.byTool) == 0 {
			b.WriteString("  (no tools)\n")
			continue
		}
		names := make([]string, 0, len(s.byTool))
		for n := range s.byTool {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, name := range names {
			t := s.byTool[name]
			desc := strings.TrimSpace(t.Description)
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Fprintf(&b, "  - %s: %s\n", name, desc)
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// Catalog returns the lightweight tool listing used in LoadMcpTools.Description.
func (r *Registry) Catalog() string {
	return r.catalog
}

// Close closes all MCP sessions in reverse connection order.
func (r *Registry) Close() error {
	if r == nil {
		return nil
	}
	var errs []error
	for i := len(r.order) - 1; i >= 0; i-- {
		if r.order[i].session != nil {
			if err := r.order[i].session.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// ToolSchemaDetail returns description and full input schema JSON for one tool.
func (r *Registry) ToolSchemaDetail(serverID, toolName string) (string, error) {
	st, ok := r.byID[serverID]
	if !ok {
		return "", fmt.Errorf("unknown mcp server %q", serverID)
	}
	t, ok := st.byTool[toolName]
	if !ok {
		return "", fmt.Errorf("unknown tool %q on server %q", toolName, serverID)
	}
	schemaJSON, err := json.MarshalIndent(t.InputSchema, "", "  ")
	if err != nil {
		schemaJSON = []byte(fmt.Sprintf("%v", t.InputSchema))
	}
	desc := strings.TrimSpace(t.Description)
	var b strings.Builder
	fmt.Fprintf(&b, "server: %q\n", serverID)
	fmt.Fprintf(&b, "tool_name: %q\n", t.Name)
	if desc != "" {
		fmt.Fprintf(&b, "description: %s\n", desc)
	}
	b.WriteString("input_schema:\n")
	b.Write(schemaJSON)
	b.WriteByte('\n')
	return b.String(), nil
}

// CallMcp invokes tools/call on the given server.
func (r *Registry) CallMcp(ctx context.Context, serverID, toolName string, arguments map[string]any) (string, error) {
	st, ok := r.byID[serverID]
	if !ok {
		return "", fmt.Errorf("unknown mcp server %q", serverID)
	}
	if _, ok := st.byTool[toolName]; !ok {
		return "", fmt.Errorf("unknown tool %q on server %q", toolName, serverID)
	}
	args := arguments
	if args == nil {
		args = map[string]any{}
	}
	res, err := st.session.CallTool(ctx, &mcpsdk.CallToolParams{Name: toolName, Arguments: args})
	if err != nil {
		return "", err
	}
	out := formatCallToolResult(res)
	if res != nil && res.IsError {
		return "", fmt.Errorf("%s", out)
	}
	return out, nil
}
