//go:build darwin || linux

package sandbox

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

func TestManagerEnforcesWorkspaceWriteBoundary(t *testing.T) {
	deps := CheckDeps()
	if !deps.AllRequiredOK() {
		missing := deps.FirstMissingRequired()
		t.Skipf("sandbox backend unavailable: %s (%s)", missing.Name, missing.Hint)
	}

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp directory: %v", err)
	}
	workspace := filepath.Join(base, "workspace")
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(
		config.SandboxConfig{Enabled: true},
		workspace,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	if !m.Enabled() {
		t.Fatal("sandbox dependencies passed but manager is not enabled")
	}

	insideMarker := filepath.Join(workspace, "inside-marker")
	outsideMarker := filepath.Join(outside, "outside-marker")
	command := "printf inside > " + strconv.Quote(insideMarker) +
		"; printf outside > " + strconv.Quote(outsideMarker)
	name, args, err := m.WrapBashCommand(context.Background(), command, "/bin/sh")
	if err != nil {
		t.Fatalf("WrapBashCommand: %v", err)
	}
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = workspace
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("sandboxed command unexpectedly wrote outside workspace; output: %s", out)
	}

	if got, err := os.ReadFile(insideMarker); err != nil || string(got) != "inside" {
		t.Fatalf("workspace write = %q, %v; want inside; command error: %v; output: %s", got, err, runErr, out)
	}
	if _, err := os.Stat(outsideMarker); !os.IsNotExist(err) {
		t.Fatalf("outside marker exists or stat failed unexpectedly: %v", err)
	}
}
