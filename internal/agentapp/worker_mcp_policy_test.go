package agentapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/icloudbb/buildmax/internal/config"
	mcpcfg "github.com/icloudbb/buildmax/internal/core/mcp"
)

// writeMCPJSON writes an mcp.json at path, creating parent directories.
func writeMCPJSON(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// stdioSideEffectConfig returns an mcp.json whose stdio command would create
// sentinelPath if it ever ran, so a test can prove the child never started.
//
// It marshals the document rather than formatting it by hand: sentinelPath is an
// OS temp path, and on Windows its backslashes would be an invalid JSON string
// escape if pasted in raw.
func stdioSideEffectConfig(id, sentinelPath string) string {
	root := mcpcfg.ConfigRoot{MCPServers: map[string]mcpcfg.ServerConfig{
		id: {Type: mcpcfg.TransportStdio, Command: "sh", Args: []string{"-c", "touch " + sentinelPath}},
	}}
	b, err := json.Marshal(root)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// A worker run whose resolved MCP config declares a stdio server fails
// construction, names the server, and never starts the child process — the
// refusal lands before NewMCPManager and before any model call.
func TestUnattendedWorkerRejectsStdioBeforeChildStarts(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	workspace := t.TempDir()
	sentinel := filepath.Join(t.TempDir(), "started")
	writeMCPJSON(t, filepath.Join(workspace, ".buildmax", "mcp.json"),
		stdioSideEffectConfig("local-fs", sentinel))

	app, err := NewAgentApp(AppConfig{WorkspaceDir: workspace, EnableMCP: true, UnattendedWorker: true})
	if err == nil {
		_ = app.Close()
		t.Fatal("worker run with a stdio MCP server must fail construction")
	}
	if !strings.Contains(err.Error(), "local-fs") {
		t.Errorf("error must name the rejected server, got: %v", err)
	}
	if !strings.Contains(err.Error(), "stdio") || !strings.Contains(err.Error(), "http") {
		t.Errorf("error must explain stdio is unsupported and name a remote transport, got: %v", err)
	}
	if _, statErr := os.Stat(sentinel); statErr == nil {
		t.Fatal("the stdio child ran: its command created the sentinel before the refusal")
	}
}

// The refusal covers every layer of the resolved configuration and sorts the
// rejected ids so the diagnostic is stable regardless of layer order.
func TestUnattendedWorkerRejectsStdioFromEveryLayer(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	workspace := t.TempDir()
	sentinel := filepath.Join(t.TempDir(), "started")

	// One stdio server per layer: global (<home>/mcp.json), workspace, plugin.
	writeMCPJSON(t, filepath.Join(home, "mcp.json"), stdioSideEffectConfig("g-global", sentinel))
	writeMCPJSON(t, filepath.Join(workspace, ".buildmax", "mcp.json"), stdioSideEffectConfig("m-workspace", sentinel))
	installPlugin(t, home, "p1", map[string]string{
		"plugin.yaml": "name: p1\n",
		"mcp.json":    stdioSideEffectConfig("a-plugin", sentinel),
	})

	app, err := NewAgentApp(AppConfig{WorkspaceDir: workspace, EnableMCP: true, UnattendedWorker: true})
	if err == nil {
		_ = app.Close()
		t.Fatal("worker run with stdio MCP servers must fail construction")
	}
	msg := err.Error()
	for _, id := range []string{"a-plugin", "g-global", "m-workspace"} {
		if !strings.Contains(msg, id) {
			t.Errorf("rejection must name %q from its layer, got: %v", id, msg)
		}
	}
	// Sorted: "a-plugin" < "g-global" < "m-workspace".
	if a, g, m := strings.Index(msg, "a-plugin"), strings.Index(msg, "g-global"), strings.Index(msg, "m-workspace"); !(a < g && g < m) {
		t.Errorf("rejected ids must be sorted for a stable diagnostic, got: %v", msg)
	}
	if _, statErr := os.Stat(sentinel); statErr == nil {
		t.Fatal("a stdio child ran before the refusal")
	}
}

// A worker keeps http and sse MCP transports: they initialize without the
// stdio refusal. Individual connection failures are non-fatal.
func TestUnattendedWorkerAllowsRemoteTransports(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	workspace := t.TempDir()
	writeMCPJSON(t, filepath.Join(workspace, ".buildmax", "mcp.json"),
		`{"mcpServers":{"h":{"type":"http","url":"http://127.0.0.1:0/mcp"},"s":{"type":"sse","url":"http://127.0.0.1:0/sse"}}}`)

	app, err := NewAgentApp(AppConfig{WorkspaceDir: workspace, EnableMCP: true, UnattendedWorker: true})
	if err != nil {
		t.Fatalf("worker http/sse MCP config must initialize, got: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
}

// A local surface (UnattendedWorker false) keeps stdio MCP: construction
// succeeds even with a stdio server declared.
func TestLocalSurfaceKeepsStdioMCP(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	workspace := t.TempDir()
	writeMCPJSON(t, filepath.Join(workspace, ".buildmax", "mcp.json"),
		`{"mcpServers":{"local":{"type":"stdio","command":"echo","args":["hi"]}}}`)

	app, err := NewAgentApp(AppConfig{WorkspaceDir: workspace, EnableMCP: true})
	if err != nil {
		t.Fatalf("local stdio MCP config must initialize, got: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
}

// rejectWorkerStdioMCP is the security contract in isolation: it never leaks
// the command or arguments, ignores remote transports, and treats an empty
// config as nothing to reject.
func TestRejectWorkerStdioMCP(t *testing.T) {
	if err := rejectWorkerStdioMCP(nil); err != nil {
		t.Errorf("nil config: want nil, got %v", err)
	}
	remoteOnly := &mcpcfg.ConfigRoot{MCPServers: map[string]mcpcfg.ServerConfig{
		"h": {Type: mcpcfg.TransportHTTP, URL: "http://example/mcp"},
		"s": {Type: mcpcfg.TransportSSE, URL: "http://example/sse"},
	}}
	if err := rejectWorkerStdioMCP(remoteOnly); err != nil {
		t.Errorf("remote-only config: want nil, got %v", err)
	}
	mixed := &mcpcfg.ConfigRoot{MCPServers: map[string]mcpcfg.ServerConfig{
		"httpsvc": {Type: mcpcfg.TransportHTTP, URL: "http://example/mcp"},
		"secretsvc": {Type: mcpcfg.TransportStdio, Command: "/usr/bin/leak",
			Args: []string{"--token", "s3cr3t"}, Env: map[string]string{"API_KEY": "s3cr3t"}},
	}}
	err := rejectWorkerStdioMCP(mixed)
	if err == nil {
		t.Fatal("mixed config with a stdio server: want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "secretsvc") {
		t.Errorf("error must name the stdio server, got: %v", msg)
	}
	if strings.Contains(msg, "httpsvc") {
		t.Errorf("error must not name a remote-transport server, got: %v", msg)
	}
	for _, leak := range []string{"leak", "s3cr3t", "API_KEY", "--token"} {
		if strings.Contains(msg, leak) {
			t.Errorf("error must not leak command/args/env %q, got: %v", leak, msg)
		}
	}
}
