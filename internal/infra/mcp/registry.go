package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	mcpcfg "github.com/gougoujiang/buildmax/internal/core/mcp"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Registry holds MCP client sessions and cached tool metadata per server.
type Registry struct {
	catalog string
	servers []*serverState
}

type serverState struct {
	id           string
	transportTyp string
	session      *mcpsdk.ClientSession
	toolsByName  map[string]*mcpsdk.Tool
}

// MCPServerStatus is one configured server's current status for diagnostics / UI.
type MCPServerStatus struct {
	ID        string
	Type      string // stdio | sse | http
	OK        bool
	Err       error // nil when OK; Error() in UI when !OK
	ToolCount int   // meaningful when OK
}

// NewRegistry connects every server in cfg, lists tools (with pagination), and builds the catalog string.
func NewRegistry(ctx context.Context, cfg *mcpcfg.ConfigRoot, httpClient *http.Client) (*Registry, error) {
	reg, rows := LoadRegistryState(ctx, cfg, httpClient)
	if reg != nil {
		return reg, nil
	}
	if len(rows) == 0 {
		return nil, errors.New("mcpservers: empty config")
	}
	for _, row := range rows {
		if row.Err != nil {
			return nil, row.Err
		}
	}
	return nil, errors.New("mcpservers: failed to initialize registry")
}

// LoadRegistryState connects to configured servers once and returns both the
// usable registry and per-server status rows. If any server fails, the returned
// registry is nil and all opened sessions are closed.
func LoadRegistryState(ctx context.Context, cfg *mcpcfg.ConfigRoot, httpClient *http.Client) (*Registry, []MCPServerStatus) {
	if cfg == nil || len(cfg.MCPServers) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(cfg.MCPServers))
	for id := range cfg.MCPServers {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	var states []*serverState
	rows := make([]MCPServerStatus, 0, len(ids))
	closeOnErr := func() {
		for i := len(states) - 1; i >= 0; i-- {
			_ = states[i].session.Close()
		}
	}

	impl := &mcpsdk.Implementation{Name: "buildmax", Version: "1"}
	for _, id := range ids {
		entry := cfg.MCPServers[id]
		row := MCPServerStatus{ID: id, Type: entry.Type}
		transport, err := newTransport(entry, httpClient)
		if err != nil {
			row.Err = fmt.Errorf("mcp server %q: %w", id, err)
			rows = append(rows, row)
			closeOnErr()
			return nil, rows
		}
		client := mcpsdk.NewClient(impl, nil)
		session, err := client.Connect(ctx, transport, nil)
		if err != nil {
			row.Err = fmt.Errorf("mcp server %q connect: %w", id, err)
			rows = append(rows, row)
			closeOnErr()
			return nil, rows
		}
		tools, err := listAllTools(ctx, session)
		if err != nil {
			_ = session.Close()
			row.Err = fmt.Errorf("mcp server %q list tools: %w", id, err)
			rows = append(rows, row)
			closeOnErr()
			return nil, rows
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
			toolsByName:  byName,
		})
		row.OK = true
		row.ToolCount = len(byName)
		rows = append(rows, row)
	}

	reg := &Registry{
		catalog: buildCatalog(states),
		servers: states,
	}
	return reg, rows
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
		if len(s.toolsByName) == 0 {
			b.WriteString("  (no tools)\n")
			continue
		}
		names := make([]string, 0, len(s.toolsByName))
		for n := range s.toolsByName {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, name := range names {
			t := s.toolsByName[name]
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
	for i := len(r.servers) - 1; i >= 0; i-- {
		if r.servers[i].session != nil {
			if err := r.servers[i].session.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// ToolSchemaDetail returns description and full input schema JSON for one tool.
func (r *Registry) ToolSchemaDetail(serverID, toolName string) (string, error) {
	st := r.lookupServer(serverID)
	if st == nil {
		return "", fmt.Errorf("unknown mcp server %q", serverID)
	}
	t, ok := st.toolsByName[toolName]
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
	st := r.lookupServer(serverID)
	if st == nil {
		return "", fmt.Errorf("unknown mcp server %q", serverID)
	}
	if _, ok := st.toolsByName[toolName]; !ok {
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
		return "", fmt.Errorf("%s", out.Text)
	}
	return out.Text, nil
}

// CallMcpMultimodal is CallMcp keeping content the text could only describe.
//
// A server that returns an image used to have it JSON-encoded into the result
// text, which sent the model a base64 blob and called it a tool result. The
// caller decides what to do with the parts; the text stays a complete,
// readable answer either way.
func (r *Registry) CallMcpMultimodal(ctx context.Context, serverID, toolName string, arguments map[string]any) (llm.ToolResult, error) {
	st := r.lookupServer(serverID)
	if st == nil {
		return llm.ToolResult{}, fmt.Errorf("unknown mcp server %q", serverID)
	}
	if _, ok := st.toolsByName[toolName]; !ok {
		return llm.ToolResult{}, fmt.Errorf("unknown tool %q on server %q", toolName, serverID)
	}
	args := arguments
	if args == nil {
		args = map[string]any{}
	}
	res, err := st.session.CallTool(ctx, &mcpsdk.CallToolParams{Name: toolName, Arguments: args})
	if err != nil {
		return llm.ToolResult{}, err
	}
	out := formatCallToolResult(res)
	if res != nil && res.IsError {
		return llm.ToolResult{}, fmt.Errorf("%s", out.Text)
	}
	return out, nil
}

// ToolIsReadOnly reports whether a server advertised readOnlyHint for one of
// its tools. Unknown server, unknown tool, and a server that says nothing all
// report false.
//
// The hint is the server's claim about itself and grants nothing — it only
// decides whether the runtime asks. In go-sdk v1.7.0 ReadOnlyHint is a bool
// rather than a pointer, so "absent" and "false" are indistinguishable after
// decode; both land on the asking side, which is the safe direction.
func (r *Registry) ToolIsReadOnly(serverID, toolName string) bool {
	st := r.lookupServer(serverID)
	if st == nil {
		return false
	}
	t, ok := st.toolsByName[toolName]
	if !ok || t == nil || t.Annotations == nil {
		return false
	}
	return t.Annotations.ReadOnlyHint
}

func (r *Registry) lookupServer(serverID string) *serverState {
	if r == nil {
		return nil
	}
	for _, srv := range r.servers {
		if srv != nil && srv.id == serverID {
			return srv
		}
	}
	return nil
}

// formatCallToolResult renders a server's result as text plus whatever the text
// could only describe.
//
// Every content kind contributes a line of text, including the ones that also
// contribute a part: a caller that drops the parts still gets a result that
// says what came back, which is what makes image content safe to send to a
// model that cannot read images.
func formatCallToolResult(res *mcpsdk.CallToolResult) llm.ToolResult {
	if res == nil {
		return llm.ToolResult{}
	}
	var lines []string
	var parts []llm.ContentPart
	for _, c := range res.Content {
		line, part := formatCallToolContent(c)
		if line != "" {
			lines = append(lines, line)
		}
		if part != nil {
			parts = append(parts, *part)
		}
	}
	s := strings.TrimSpace(strings.Join(lines, "\n"))
	if res.StructuredContent != nil {
		b, err := json.MarshalIndent(res.StructuredContent, "", "  ")
		if err == nil && len(b) > 0 {
			if s != "" {
				s += "\n\n"
			}
			s += "structured_content:\n" + string(b)
		}
	}
	if s == "" && !res.IsError {
		s = "(empty tool result)"
	}
	return llm.ToolResult{Text: strings.TrimSpace(s), Parts: parts}
}

// formatCallToolContent renders one content item as a line of text and, when it
// is something text cannot hold, a part carrying it.
func formatCallToolContent(c mcpsdk.Content) (string, *llm.ContentPart) {
	switch x := c.(type) {
	case *mcpsdk.TextContent:
		return x.Text, nil
	case *mcpsdk.ImageContent:
		if x.Data == nil || x.MIMEType == "" {
			break
		}
		encoded := base64.StdEncoding.EncodeToString(x.Data)
		return fmt.Sprintf("(image: %s, %s)", x.MIMEType, byteSize(len(x.Data))),
			&llm.ContentPart{
				Type:      llm.ContentPartImage,
				MediaType: x.MIMEType,
				Data:      encoded,
			}
	}
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Sprintf("%v", c), nil
	}
	return string(b), nil
}

// byteSize renders a length the way a person reads it, so a result line says
// how big an image was rather than how many bytes it had.
func byteSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
