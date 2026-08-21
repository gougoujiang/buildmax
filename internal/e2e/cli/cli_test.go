package clie2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/testsupport/mockllm"
)

// binary is built once for the whole suite: the boundary under test is the
// released binary, and rebuilding it per test would spend the budget in
// docs/design/end-to-end-testing.md §5.1 on the compiler.
var binary string

func TestMain(m *testing.M) {
	root, err := moduleRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dir, err := os.MkdirTemp("", "buildmax-cli-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "buildmax")
	if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr != nil {
		fmt.Fprintln(os.Stderr, statErr)
		os.Exit(1)
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/buildmax")
	build.Dir = root
	if out, buildErr := build.CombinedOutput(); buildErr != nil {
		fmt.Fprintf(os.Stderr, "build buildmax: %v\n%s", buildErr, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// TestAPinnedWriteRunsAndReportsBack is the CLI golden path: the model asks for
// a write, the write happens in the workspace, and the model's own summary of
// it reaches stdout. Nothing below this level assembles all three.
func TestAPinnedWriteRunsAndReportsBack(t *testing.T) {
	server := startModel(t, "write-a-file.json")
	workspace := t.TempDir()
	home := writeHome(t, server, map[string]string{"Write": "allow"})

	result := run(t, home, workspace, "-p", "write notes.txt", "--output", "jsonl")

	if result.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", result.exitCode, result.stdout, result.stderr)
	}
	written, err := os.ReadFile(filepath.Join(workspace, "notes.txt"))
	if err != nil {
		t.Fatalf("the scripted write did not reach the workspace: %v", err)
	}
	if string(written) != "scripted content\n" {
		t.Fatalf("file content = %q, want the scripted content", written)
	}
	if reply := result.field("result", "reply"); reply != "wrote notes.txt" {
		t.Fatalf("reply = %q, want the model's closing text", reply)
	}
	if ended := result.events("tool_end"); len(ended) != 1 || ended[0]["tool"] != "Write" {
		t.Fatalf("tool_end events = %v, want one Write", ended)
	}
	if denied := result.events("tool_denied"); len(denied) != 0 {
		t.Fatalf("tool_denied events = %v, want none for a pinned allow", denied)
	}
	// The run has to have consumed the whole script. A run that stopped a turn
	// early would still have written the file and still look like a pass.
	if remaining := server.Remaining(); remaining != 0 {
		t.Fatalf("unconsumed scenario steps = %d, want 0", remaining)
	}
}

// TestAPinnedAskIsRefusedWithoutAHuman covers the other half of the gate: print
// mode has no approval handler, so a configured Ask is a refusal rather than a
// prompt nobody can answer. The run still finishes, and the file stays absent.
func TestAPinnedAskIsRefusedWithoutAHuman(t *testing.T) {
	server := startModel(t, "write-a-file.json")
	workspace := t.TempDir()
	home := writeHome(t, server, map[string]string{"Write": "ask"})

	result := run(t, home, workspace, "-p", "write notes.txt", "--output", "jsonl")

	if _, err := os.Stat(filepath.Join(workspace, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("a refused write must not touch the workspace (stat err = %v)", err)
	}
	denied := result.events("tool_denied")
	if len(denied) != 1 || denied[0]["tool"] != "Write" {
		t.Fatalf("tool_denied events = %v, want one Write\nstdout:\n%s", denied, result.stdout)
	}
	if reason, _ := denied[0]["reason"].(string); reason == "" {
		t.Fatalf("denial carries no reason: %v", denied[0])
	}
	if remaining := server.Remaining(); remaining != 0 {
		t.Fatalf("unconsumed scenario steps = %d, want 0", remaining)
	}
}

func startModel(t *testing.T, scenarioFile string) *mockllm.Server {
	t.Helper()
	scenario, err := mockllm.LoadScenario(filepath.Join("testdata", scenarioFile))
	if err != nil {
		t.Fatalf("load scenario: %v", err)
	}
	server, err := mockllm.Start(scenario)
	if err != nil {
		t.Fatalf("start mockllm: %v", err)
	}
	t.Cleanup(server.Close)
	return server
}

// writeHome builds the throwaway BUILDMAX_HOME a run reads: one model entry
// pointing at the scripted server, and the permissions the test depends on
// written down rather than inherited from whatever the tool defaults to today.
func writeHome(t *testing.T, server *mockllm.Server, permissions map[string]string) string {
	t.Helper()
	home := t.TempDir()
	settings := strings.Builder{}
	settings.WriteString("log_level: error\n")
	settings.WriteString("models:\n")
	settings.WriteString("  - model: mock-model\n")
	settings.WriteString("    name: mock\n")
	fmt.Fprintf(&settings, "    api_url: %q\n", server.BaseURL(mockllm.ProtocolOpenAIChat))
	settings.WriteString("    api_key: mock-key\n")
	settings.WriteString("    context_window: 128000\n")
	if len(permissions) > 0 {
		settings.WriteString("tools:\n  permissions:\n")
		for tool, action := range permissions {
			fmt.Fprintf(&settings, "    %s: %s\n", tool, action)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "settings.yaml"), []byte(settings.String()), 0o600); err != nil {
		t.Fatalf("write settings.yaml: %v", err)
	}
	return home
}

type runResult struct {
	stdout   string
	stderr   string
	exitCode int
	lines    []map[string]any
}

// events returns the jsonl records of one type, in order.
func (r runResult) events(kind string) []map[string]any {
	var out []map[string]any
	for _, line := range r.lines {
		if line["type"] == kind {
			out = append(out, line)
		}
	}
	return out
}

// field reads one field of the first record of a type, as a string.
func (r runResult) field(kind, name string) string {
	for _, line := range r.lines {
		if line["type"] == kind {
			s, _ := line[name].(string)
			return s
		}
	}
	return ""
}

func run(t *testing.T, home, workspace string, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(binary, append(args, "--workspace", workspace)...)
	cmd.Env = append(os.Environ(), "BUILDMAX_HOME="+home)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", binary, err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		result := runResult{stdout: stdout.String(), stderr: stderr.String()}
		var exitErr *exec.ExitError
		switch {
		case err == nil:
		case asExitError(err, &exitErr):
			result.exitCode = exitErr.ExitCode()
		default:
			t.Fatalf("run %s: %v", binary, err)
		}
		result.lines = parseJSONL(result.stdout)
		return result
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("the run did not finish in 30s\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
		return runResult{}
	}
}

func asExitError(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		*target = exitErr
	}
	return ok
}

// parseJSONL keeps only the records: anything else on stdout is log noise that
// a failure message should still show, but that no assertion depends on.
func parseJSONL(out string) []map[string]any {
	var lines []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var record map[string]any
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue
		}
		if _, ok := record["type"]; ok {
			lines = append(lines, record)
		}
	}
	return lines
}
