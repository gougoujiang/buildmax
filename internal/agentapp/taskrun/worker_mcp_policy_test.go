package taskrun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/icloudbb/buildmax/internal/config"
	coretask "github.com/icloudbb/buildmax/internal/core/task"
	"github.com/icloudbb/buildmax/internal/testsupport/mockllm"
)

// A worker run whose workspace declares a stdio MCP server fails before the
// model is ever called: the unattended-worker profile disables the transport,
// and the refusal lands during runtime assembly, not after a turn.
func TestWorkerRunRejectsStdioMCPBeforeModelCall(t *testing.T) {
	ctx := context.Background()
	server, err := mockllm.Start(mockllm.Scenario{Steps: []mockllm.Step{{Text: "should never run"}}})
	if err != nil {
		t.Fatalf("start mock model: %v", err)
	}
	t.Cleanup(server.Close)
	model := config.ModelEntry{
		Model: "mock-model", Name: "mock", APIURL: server.BaseURL(mockllm.ProtocolOpenAIChat),
		APIKey: "mock-key", ContextWindow: 128000,
	}

	dirs := testRunDirs(t)
	sentinel := filepath.Join(t.TempDir(), "started")
	mcpPath := filepath.Join(dirs.runWorkspace, ".buildmax", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"mcpServers":{"fs":{"type":"stdio","command":"sh","args":["-c","touch ` + sentinel + `"]}}}`
	if err := os.WriteFile(mcpPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	run := &coretask.Run{ID: "run1", Input: "do the work"}
	_, err = runAgentTask(ctx, run, dirs.runWorkspace, dirs.runGlobal, dirs.runOSHome,
		"sid-worker", nil, model, ManagedInference{}, nil, "", "", nil, nil, "", "", nil)
	if err == nil {
		t.Fatal("worker run with a stdio MCP server must fail")
	}
	if !strings.Contains(err.Error(), "stdio") || !strings.Contains(err.Error(), "fs") {
		t.Errorf("failure must name the policy and the rejected server, got: %v", err)
	}
	if len(server.Requests()) != 0 {
		t.Errorf("the model must not be called: got %d requests", len(server.Requests()))
	}
	if _, statErr := os.Stat(sentinel); statErr == nil {
		t.Fatal("the stdio child ran before the refusal")
	}
}
