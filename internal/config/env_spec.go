package config

import "strings"

// This file is the single source of truth for environment variable names and the few
// functions that read them. Most configuration now lives in settings.yaml (local) or
// server.yaml (server/worker). Only bootstrap-level values that cannot be in a file
// remain as environment variables.

// Environment variable key names.
const (
	// BUILDMAX_HOME — path to the data directory; tells every binary where to find its
	// config files (settings.yaml, server.yaml). Must remain an env var because the
	// config files cannot be located until this path is known.
	EnvKeyBuildmaxHome = "BUILDMAX_HOME"

	// BUILDMAX_JWT_SECRET — optional override for jwt_secret in server.yaml.
	// In production, inject via env var (Kubernetes Secret, Docker secret) instead of
	// storing the value in the YAML file on disk.
	EnvKeyBuildmaxJWTSecret = "BUILDMAX_JWT_SECRET"

	// Test only.
	EnvKeyBuildmaxTestDSN = "BUILDMAX_TEST_DSN"
)

// Note: EnvKeyBuildmaxSandboxEnabled lives in sandbox.go alongside the
// resolver that reads it (per-subsystem env constant).

// EnvVar describes one environment variable used by BuildMax.
type EnvVar struct {
	Name        string
	Default     string
	Description string
	// WorkerNeeds marks a variable that a task-run worker reads.
	//
	// A worker executes model-chosen code, so a credential it does not read is
	// exposure with no purpose behind it. Only the marked variables are handed
	// to a worker process or Job pod. internal/bootstrap/worker.go is the
	// authority on what that set is: if a worker starts reading a new variable,
	// mark it here, and it will reach the worker. Nothing else will.
	WorkerNeeds bool
}

// EnvVars lists every environment variable read by BuildMax binaries.
var EnvVars = []EnvVar{
	{Name: EnvKeyBuildmaxHome, Default: "~/.buildmax", Description: "Application data directory; locates settings.yaml and server.yaml", WorkerNeeds: true},
	{Name: EnvKeyBuildmaxJWTSecret, Description: "Override for jwt_secret in server.yaml; inject at deploy time in production"},
	{Name: EnvKeyBuildmaxDatabasePassword, Description: "Override for database.password in server.yaml"},
	{Name: EnvKeyBuildmaxMinIOAccessKey, Description: "Override for storage.minio.access_key in server.yaml", WorkerNeeds: true},
	{Name: EnvKeyBuildmaxMinIOSecretKey, Description: "Override for storage.minio.secret_key in server.yaml", WorkerNeeds: true},
	{Name: EnvKeyBuildmaxWorkerToken, Description: "Override for worker.token in server.yaml; shared secret for /api/worker/*", WorkerNeeds: true},
	{Name: EnvKeyBuildmaxConversationAPIKey, Description: "Override for conversation.model.api_key in server.yaml", WorkerNeeds: true},
	{Name: EnvKeyBuildmaxTestDSN, Description: "MySQL DSN for store integration tests; unset skips those tests"},
	{Name: EnvKeyBuildmaxSandboxEnabled, Description: "Override sandbox.enabled in settings; values: 1/true/yes/on or 0/false/no/off", WorkerNeeds: true},
	{Name: EnvKeyBuildmaxTraceDisabled, Description: "Disable durable run traces when truthy (1/true/yes/on); traces are on by default", WorkerNeeds: true},
}

// WorkerNeedsEnv reports whether a task-run worker reads name.
//
// It answers false for an unrecognized BUILDMAX_ variable as well as for a
// known one a worker does not use, so a variable added to the server without a
// thought for workers stays on the server.
func WorkerNeedsEnv(name string) bool {
	for _, v := range EnvVars {
		if v.Name == name {
			return v.WorkerNeeds
		}
	}
	return false
}

// WorkerEnvKeys returns the environment variable names a worker is given, in
// declaration order.
func WorkerEnvKeys() []string {
	var out []string
	for _, v := range EnvVars {
		if v.WorkerNeeds {
			out = append(out, v.Name)
		}
	}
	return out
}

// FilterWorkerEnv returns environ with the BUILDMAX_ variables a worker does
// not read removed.
//
// Everything outside the BUILDMAX_ prefix passes through untouched: PATH, HOME
// and the rest are what let the worker binary run at all. Use this when handing
// a local worker process the server's environment.
func FilterWorkerEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, e := range environ {
		name, _, found := strings.Cut(e, "=")
		if found && strings.HasPrefix(name, "BUILDMAX_") && !WorkerNeedsEnv(name) {
			continue
		}
		out = append(out, e)
	}
	return out
}
