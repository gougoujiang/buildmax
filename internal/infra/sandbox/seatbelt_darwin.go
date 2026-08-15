//go:build darwin

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
)

func newBackend(name string) (backend, error) {
	if name != "seatbelt" {
		return nil, errors.New("sandbox: only seatbelt is supported on darwin")
	}
	path, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "buildmax-sandbox-")
	if err != nil {
		return nil, fmt.Errorf("sandbox: profile dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("sandbox: chmod profile dir: %w", err)
	}
	return &seatbeltBackend{path: path, profileDir: dir}, nil
}

// seatbeltBackend wraps bash via macOS sandbox-exec
// (https://www.unix.com/man-page/mojave/1/sandbox-exec/). The TinyScheme
// profile is generated per call so it picks up settings updates without
// the manager having to cache profile state.
//
// Phase B enforces filesystem isolation only: deny writes outside the
// workspace and the explicit allow_write list; deny reads of the explicit
// deny_read list. Network egress is still open in Phase B (Phase C adds
// the proxy + deny-by-default network rule). Env scrubbing lands in
// Phase D.
type seatbeltBackend struct {
	path       string
	profileDir string
}

func (s *seatbeltBackend) Name() string { return "seatbelt" }

func (s *seatbeltBackend) Wrap(_ context.Context, p WrapParams) (string, []string, error) {
	profile := buildSeatbeltProfile(p)
	f, err := os.CreateTemp(s.profileDir, "profile-*.sb")
	if err != nil {
		return "", nil, fmt.Errorf("sandbox: write profile: %w", err)
	}
	if _, err := f.WriteString(profile); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", nil, fmt.Errorf("sandbox: write profile: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	// HTTP_PROXY env is set on cmd.Env by the bash tool itself
	// (sandbox.SandboxView.ChildEnv) — sandbox-exec's -D flag only
	// substitutes profile parameters and does not affect child env.
	args := []string{"-f", f.Name(), shellOrDefault(p.Shell), "-c", p.Command}
	return s.path, args, nil
}

func (s *seatbeltBackend) Close() error {
	if s.profileDir == "" {
		return nil
	}
	return os.RemoveAll(s.profileDir)
}

// buildSeatbeltProfile generates a Seatbelt profile (TinyScheme syntax)
// from WrapParams. Extracted so a golden test can exercise it.
func buildSeatbeltProfile(p WrapParams) string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	// Default: allow most operations but lock down filesystem writes.
	// Matches the upstream "permissive base, deny what we care about"
	// approach so commands like `git status` and `go test` keep working.
	b.WriteString("(allow default)\n")
	b.WriteString("(deny file-write*)\n")

	// Workspace is always writable.
	fmt.Fprintf(&b, "(allow file-write* (subpath %s))\n", quoteSeatbelt(p.Workspace))
	// Additional writable paths from settings.
	for _, w := range expandSeatbeltPaths(p.Cfg.Filesystem.AllowWrite) {
		fmt.Fprintf(&b, "(allow file-write* (subpath %s))\n", quoteSeatbelt(w))
	}
	// Deny-write: re-deny after allow so explicit denies always win.
	for _, w := range expandSeatbeltPaths(p.Cfg.Filesystem.DenyWrite) {
		fmt.Fprintf(&b, "(deny file-write* (subpath %s))\n", quoteSeatbelt(w))
	}
	// Deny-read entries shadow specific paths.
	for _, r := range expandSeatbeltPaths(p.Cfg.Filesystem.DenyRead) {
		fmt.Fprintf(&b, "(deny file-read* (subpath %s))\n", quoteSeatbelt(r))
	}
	// Allow-read re-allows within a denied region.
	for _, r := range expandSeatbeltPaths(p.Cfg.Filesystem.AllowRead) {
		fmt.Fprintf(&b, "(allow file-read* (subpath %s))\n", quoteSeatbelt(r))
	}
	// Network egress: deny everything, then allow only the in-process
	// proxy on loopback. Seatbelt's network-address clauses accept "*"
	// or "localhost" as the host (numeric IPs are rejected) — the
	// kernel still applies the rule by socket address, so localhost
	// covers 127.0.0.1.
	if p.ProxyAddr != "" {
		_, port := splitHostPort(p.ProxyAddr)
		if port != "" {
			b.WriteString("(deny network-outbound)\n")
			fmt.Fprintf(&b, "(allow network-outbound (remote tcp \"localhost:%s\"))\n", port)
			// Allow DNS resolution so cooperating tools can look up
			// hostnames before handing them to the proxy.
			b.WriteString("(allow network-outbound (remote udp \"*:53\"))\n")
			b.WriteString("(allow network-outbound (remote tcp \"*:53\"))\n")
			// Local Unix-socket I/O (syslog, abstract sockets) stays
			// available; without this many commands break in subtle
			// ways under sandbox-exec.
			b.WriteString("(allow network-outbound (remote unix-socket))\n")
		}
	}
	return b.String()
}

// quoteSeatbelt wraps a path in double quotes and escapes existing quotes.
// Seatbelt's TinyScheme syntax does not support `\\` escapes for spaces,
// so callers must pass absolute paths. We never accept arbitrary user
// text here — only resolved filesystem paths from settings.
func quoteSeatbelt(path string) string {
	// Defensive: drop NUL and reject empty.
	clean := strings.ReplaceAll(path, "\x00", "")
	return `"` + strings.ReplaceAll(clean, `"`, `\"`) + `"`
}

// expandSeatbeltPaths is the macOS-side path normaliser for Phase B. It
// drops empty entries and converts `~` to the user home. Other path
// semantics (relative, `./`) are left for Phase D's full resolver.
func expandSeatbeltPaths(in []string) []string {
	out := make([]string, 0, len(in))
	home, _ := os.UserHomeDir()
	for _, p := range in {
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "~/") && home != "" {
			p = filepath.Join(home, p[2:])
		} else if p == "~" && home != "" {
			p = home
		}
		out = append(out, p)
	}
	return out
}

// _ keeps the config import live.
var _ = config.SandboxConfig{}
