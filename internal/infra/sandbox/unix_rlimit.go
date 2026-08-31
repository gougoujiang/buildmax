package sandbox

import (
	"strconv"

	"github.com/gougoujiang/buildmax/internal/config"
)

// ulimitPrefix returns shell statements enforcing cfg's process limits,
// prepended to the command both backends already run as
// `/bin/sh -c <command>`. Empty when no limit is set.
//
// A shell builtin, not a Go-side syscall.Setrlimit call: os/exec.Cmd has no
// pre-exec hook that reaches only the spawned child and not the parent
// buildmax process, while both backends already hand the whole command to a
// shell, which reaches exactly the sandboxed child on every platform this
// package supports. See config.SandboxProcessConfig's own comment.
//
// Each limit is its own `ulimit` statement, not one call with several flags:
// ulimit -v is unsupported on macOS (Darwin's setrlimit has no RLIMIT_AS)
// and fails outright there, and one failing flag must not stop the others in
// the same call from ever running. Failures are silenced (2>/dev/null)
// rather than surfaced: a limit this layer cannot apply is not a reason to
// refuse to run the command at all, the same restraint excluded_commands
// documents for a convenience rather than a hard boundary.
func ulimitPrefix(cfg config.SandboxConfig) string {
	var out string
	if n := cfg.Process.MaxCPUSeconds; n > 0 {
		out += "ulimit -t " + strconv.Itoa(n) + " 2>/dev/null; "
	}
	if mb := cfg.Process.MaxMemoryMB; mb > 0 {
		out += "ulimit -v " + strconv.Itoa(mb*1024) + " 2>/dev/null; "
	}
	if n := cfg.Process.MaxProcesses; n > 0 {
		out += "ulimit -u " + strconv.Itoa(n) + " 2>/dev/null; "
	}
	if n := cfg.Process.MaxOpenFiles; n > 0 {
		out += "ulimit -n " + strconv.Itoa(n) + " 2>/dev/null; "
	}
	return out
}
