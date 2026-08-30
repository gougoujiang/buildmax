//go:build darwin

package sandbox

import (
	"context"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

// TestSeatbeltProfile_Golden asserts the generated profile is the shape
// we expect for the documented baseline. The profile is the contract
// between Phase B and macOS' Seatbelt runtime — drift here typically
// means commands inside the sandbox lose access they need.
func TestSeatbeltProfile_Golden(t *testing.T) {
	p := WrapParams{
		Command:   "echo hi",
		Shell:     "/bin/bash",
		Workspace: "/Users/dev/proj",
		Cfg: config.SandboxConfig{
			Enabled: true,
			Filesystem: config.SandboxFSConfig{
				AllowWrite: []string{"/tmp/build", "~/.cache"},
				DenyWrite:  []string{"/Users/dev/proj/secrets"},
				DenyRead:   []string{"~/.aws"},
				AllowRead:  []string{"/Users/dev/proj/allowed"},
			},
		},
	}
	got := buildSeatbeltProfile(p)
	wantContains := []string{
		"(version 1)",
		"(allow default)",
		"(deny file-write*)",
		`(allow file-write* (subpath "/Users/dev/proj"))`,
		`(allow file-write* (subpath "/tmp/build"))`,
		`(deny file-write* (subpath "/Users/dev/proj/secrets"))`,
		`(allow file-read* (subpath "/Users/dev/proj/allowed"))`,
	}
	for _, want := range wantContains {
		if !strings.Contains(got, want) {
			t.Errorf("profile missing %q:\n---\n%s\n---", want, got)
		}
	}
	// allow_write order must come before deny_write of overlapping paths.
	allowIdx := strings.Index(got, `(allow file-write* (subpath "/Users/dev/proj"))`)
	denyIdx := strings.Index(got, `(deny file-write* (subpath "/Users/dev/proj/secrets"))`)
	if allowIdx < 0 || denyIdx < 0 || allowIdx > denyIdx {
		t.Errorf("ordering: allow=%d deny=%d (want allow before deny)", allowIdx, denyIdx)
	}
}

// TestSeatbeltProfile_NetworkDeniedWhenProxy asserts the profile denies
// network-outbound and allow-lists only the proxy port.
func TestSeatbeltProfile_NetworkDeniedWhenProxy(t *testing.T) {
	p := WrapParams{
		Command:   "curl example.com",
		Workspace: "/tmp/ws",
		ProxyAddr: "127.0.0.1:54321",
	}
	got := buildSeatbeltProfile(p)
	want := []string{
		"(deny network-outbound)",
		`(allow network-outbound (remote tcp "localhost:54321"))`,
		`(allow network-outbound (remote udp "*:53"))`,
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("profile missing %q:\n%s", w, got)
		}
	}
}

// TestSeatbeltProfile_NetworkOpenWithoutProxy asserts no network restriction
// when no proxy is running (graceful degradation).
func TestSeatbeltProfile_NetworkOpenWithoutProxy(t *testing.T) {
	p := WrapParams{Command: "curl example.com", Workspace: "/tmp/ws"}
	got := buildSeatbeltProfile(p)
	if strings.Contains(got, "(deny network-outbound)") {
		t.Errorf("profile unexpectedly denies network without proxy:\n%s", got)
	}
}

// TestSeatbeltProfile_HomeExpansion asserts ~/x expands to $HOME/x.
func TestSeatbeltProfile_HomeExpansion(t *testing.T) {
	t.Setenv("HOME", "/Users/test")
	p := WrapParams{
		Command:   "id",
		Workspace: "/tmp/ws",
		Cfg: config.SandboxConfig{
			Enabled: true,
			Filesystem: config.SandboxFSConfig{
				DenyRead: []string{"~/.aws"},
			},
		},
	}
	got := buildSeatbeltProfile(p)
	if !strings.Contains(got, `(deny file-read* (subpath "/Users/test/.aws"))`) {
		t.Errorf("~ expansion missing:\n%s", got)
	}
}

// TestSeatbeltWrap_ProcessLimitsPrefixTheCommand asserts a configured
// process limit becomes a `ulimit` statement prefixed onto the inner
// shell's -c argument, ahead of the user's own command — Seatbelt's .sb
// profile grammar has no resource-limit primitive of its own
// (docs/design/sandbox-boundaries.md §7.1), so this is the only mechanism
// on macOS.
func TestSeatbeltWrap_ProcessLimitsPrefixTheCommand(t *testing.T) {
	b := &seatbeltBackend{path: "sandbox-exec", profileDir: t.TempDir()}
	p := WrapParams{
		Command:   "id",
		Workspace: "/tmp/ws",
		Cfg: config.SandboxConfig{
			Process: config.SandboxProcessConfig{MaxCPUSeconds: 10, MaxOpenFiles: 64},
		},
	}
	_, args, err := b.Wrap(context.Background(), p)
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	last := args[len(args)-1]
	if !strings.HasPrefix(last, "ulimit -t 10 2>/dev/null; ulimit -n 64 2>/dev/null; ") {
		t.Fatalf("-c argument = %q, want it to start with the ulimit statements", last)
	}
	if !strings.HasSuffix(last, "id") {
		t.Errorf("-c argument = %q, want the user's own command last", last)
	}
}
