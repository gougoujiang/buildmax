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

	// BUILDMAX_RUN_TOKEN — the credential one task run presents to the managed LLM
	// gateway. The scheduler mints it per run and puts it in the worker process or
	// Job pod; nothing inherits it, which is why it is not marked WorkerNeeds
	// below. See docs/design/worker-run-token.md.
	EnvKeyBuildmaxRunToken = "BUILDMAX_RUN_TOKEN"

	// Test only.
	EnvKeyBuildmaxTestDSN = "BUILDMAX_TEST_DSN"

	// BUILDMAX_CACHE_QUALIFY_* name the provider the prompt-cache qualification
	// suite runs against. The suite calls a real, paid provider and is not part
	// of any check; unset, it skips. See docs/design/prompt-cache-control.md
	// section 9, phase 4.
	EnvKeyBuildmaxCacheQualifyProvider = "BUILDMAX_CACHE_QUALIFY_PROVIDER"
	EnvKeyBuildmaxCacheQualifyModel    = "BUILDMAX_CACHE_QUALIFY_MODEL"
	EnvKeyBuildmaxCacheQualifyAPIKey   = "BUILDMAX_CACHE_QUALIFY_API_KEY"
	EnvKeyBuildmaxCacheQualifyBaseURL  = "BUILDMAX_CACHE_QUALIFY_BASE_URL"
	// BUILDMAX_CACHE_QUALIFY_SLOW opts into the scenarios that must wait out a
	// retention window. They take minutes of wall clock, so they are off by
	// default rather than silently making a run look hung.
	EnvKeyBuildmaxCacheQualifySlow = "BUILDMAX_CACHE_QUALIFY_SLOW"
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
	// DirectLLMOnly narrows WorkerNeeds to deployments whose task runs call a
	// provider themselves. A managed run reaches models through the server and
	// holds no provider credential, so passing one would hand a model-driven
	// process a key it has no use for.
	DirectLLMOnly bool
}

// EnvVars lists every environment variable read by BuildMax binaries.
var EnvVars = []EnvVar{
	{Name: EnvKeyBuildmaxHome, Default: "~/.buildmax", Description: "Application data directory; locates settings.yaml and server.yaml", WorkerNeeds: true},
	{Name: EnvKeyBuildmaxJWTSecret, Description: "Override for jwt_secret in server.yaml; inject at deploy time in production"},
	{Name: EnvKeyBuildmaxDatabasePassword, Description: "Override for database.password in server.yaml"},
	{Name: EnvKeyBuildmaxMinIOAccessKey, Description: "Override for storage.minio.access_key in server.yaml", WorkerNeeds: true},
	{Name: EnvKeyBuildmaxMinIOSecretKey, Description: "Override for storage.minio.secret_key in server.yaml", WorkerNeeds: true},
	{Name: EnvKeyBuildmaxWorkerToken, Description: "Override for worker.token in server.yaml; shared secret for /api/worker/*", WorkerNeeds: true},
	{Name: EnvKeyBuildmaxConversationAPIKey, Description: "Override for conversation.model.api_key in server.yaml", WorkerNeeds: true, DirectLLMOnly: true},
	// Deliberately not WorkerNeeds: a worker reads this, but it is injected per
	// run by the scheduler, never inherited from the server. Leaving it unmarked
	// is what strips a stale value the server happens to be holding, so the only
	// token a worker can find is the one minted for its own run.
	{Name: EnvKeyBuildmaxRunToken, Description: "Per-run credential for the managed LLM gateway; minted by the scheduler, not set by an operator"},
	{Name: EnvKeyBuildmaxTestDSN, Description: "MySQL DSN for store integration tests; unset skips those tests"},
	{Name: EnvKeyBuildmaxCacheQualifyProvider, Description: "Provider for the prompt-cache qualification suite; unset skips it"},
	{Name: EnvKeyBuildmaxCacheQualifyModel, Description: "Model identifier for the prompt-cache qualification suite"},
	{Name: EnvKeyBuildmaxCacheQualifyAPIKey, Description: "Credential for the prompt-cache qualification suite; a real, paid provider"},
	{Name: EnvKeyBuildmaxCacheQualifyBaseURL, Description: "Base URL override for the prompt-cache qualification suite"},
	{Name: EnvKeyBuildmaxCacheQualifySlow, Description: "Include the qualification scenarios that wait out a retention window (1/true/yes/on)"},
	{Name: EnvKeyBuildmaxSandboxEnabled, Description: "Override sandbox.enabled in settings; values: 1/true/yes/on or 0/false/no/off", WorkerNeeds: true},
	{Name: EnvKeyBuildmaxTraceDisabled, Description: "Disable durable run traces when truthy (1/true/yes/on); traces are on by default", WorkerNeeds: true},
}

// WorkerNeedsEnv reports whether a task-run worker reads name.
//
// managedLLM says whether this deployment's task runs reach models through the
// server. When they do, provider credentials are withheld: the run has a
// gateway credential instead and never calls a provider itself.
//
// It answers false for an unrecognized BUILDMAX_ variable as well as for a
// known one a worker does not use, so a variable added to the server without a
// thought for workers stays on the server.
func WorkerNeedsEnv(name string, managedLLM bool) bool {
	for _, v := range EnvVars {
		if v.Name == name {
			return v.WorkerNeeds && !(managedLLM && v.DirectLLMOnly)
		}
	}
	return false
}

// WorkerEnvKeys returns the environment variable names a worker is given, in
// declaration order.
func WorkerEnvKeys(managedLLM bool) []string {
	var out []string
	for _, v := range EnvVars {
		if WorkerNeedsEnv(v.Name, managedLLM) {
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
func FilterWorkerEnv(environ []string, managedLLM bool) []string {
	out := make([]string, 0, len(environ))
	for _, e := range environ {
		name, _, found := strings.Cut(e, "=")
		if found && strings.HasPrefix(name, "BUILDMAX_") && !WorkerNeedsEnv(name, managedLLM) {
			continue
		}
		out = append(out, e)
	}
	return out
}
