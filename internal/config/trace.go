package config

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvKeyBuildmaxTraceDisabled, when truthy, turns off durable run traces.
// Per-subsystem env constant (kept here next to the resolver, mirroring the
// sandbox convention) and registered in env_spec.go EnvVars.
const EnvKeyBuildmaxTraceDisabled = "BUILDMAX_TRACE_DISABLED"

// TracesDir returns the directory holding durable run traces under DataDir.
// Layout: <DataDir>/traces/<session_id>/<run_id>.jsonl. Does not create the
// directory; the recorder creates it on demand.
func TracesDir() string {
	return filepath.Join(DataDir(), "traces")
}

// TraceEnabled reports whether durable run traces should be written. Traces are
// on by default; set BUILDMAX_TRACE_DISABLED to a truthy value (1/true/yes/on)
// to turn them off.
func TraceEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvKeyBuildmaxTraceDisabled))) {
	case "1", "true", "yes", "on":
		return false
	default:
		return true
	}
}
