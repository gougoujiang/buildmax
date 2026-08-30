package sandbox

import (
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

// TestUlimitPrefix_Empty asserts no limits set means no prefix at all, so an
// unconfigured deployment's wrapped command is byte-identical to before this
// field existed.
func TestUlimitPrefix_Empty(t *testing.T) {
	if got := ulimitPrefix(config.SandboxConfig{}); got != "" {
		t.Errorf("ulimitPrefix(zero value) = %q, want empty", got)
	}
}

// TestUlimitPrefix_EachLimitOwnStatement asserts every set limit becomes its
// own `ulimit` statement rather than one call with several flags: ulimit -v
// fails outright on macOS, and one failing flag must not stop the others in
// the same call from ever running.
func TestUlimitPrefix_EachLimitOwnStatement(t *testing.T) {
	got := ulimitPrefix(config.SandboxConfig{Process: config.SandboxProcessConfig{
		MaxCPUSeconds: 30,
		MaxMemoryMB:   512,
		MaxProcesses:  64,
		MaxOpenFiles:  256,
	}})
	statements := strings.Split(strings.TrimSpace(got), ";")
	// Trailing split artifact from the final "; " is an empty string; trim it.
	if len(statements) > 0 && strings.TrimSpace(statements[len(statements)-1]) == "" {
		statements = statements[:len(statements)-1]
	}
	if len(statements) != 4 {
		t.Fatalf("statements = %v (from %q), want 4 separate ulimit calls", statements, got)
	}
	mustContain := []string{"ulimit -t 30", "ulimit -v 524288", "ulimit -u 64", "ulimit -n 256"}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Errorf("prefix %q missing %q", got, want)
		}
	}
	// MaxMemoryMB is converted to KB (ulimit -v's unit), not passed through raw.
	if strings.Contains(got, "ulimit -v 512 ") {
		t.Errorf("MaxMemoryMB was not converted from MB to KB: %q", got)
	}
	// Each statement silences its own failure rather than letting one
	// unsupported flag (ulimit -v on macOS) abort the ones after it.
	for _, s := range statements {
		if !strings.Contains(s, "2>/dev/null") {
			t.Errorf("statement %q does not silence its own failure", s)
		}
	}
}

// TestUlimitPrefix_OneLimit asserts a single configured limit does not pull
// in statements for the others.
func TestUlimitPrefix_OneLimit(t *testing.T) {
	got := ulimitPrefix(config.SandboxConfig{Process: config.SandboxProcessConfig{MaxOpenFiles: 128}})
	if got != "ulimit -n 128 2>/dev/null; " {
		t.Errorf("ulimitPrefix = %q, want exactly the one statement", got)
	}
}
