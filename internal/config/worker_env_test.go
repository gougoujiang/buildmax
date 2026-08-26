package config

import (
	"slices"
	"strings"
	"testing"
)

// TestWorkerEnv_WithholdsServerOnlyCredentials is the point of the whole
// mechanism. A worker executes model-chosen shell commands; the JWT signing
// secret would let one mint a token for any user, and the database password
// would give it every team's data. internal/bootstrap/worker.go reads neither.
func TestWorkerEnv_WithholdsServerOnlyCredentials(t *testing.T) {
	withheld := []string{
		EnvKeyBuildmaxJWTSecret,
		EnvKeyBuildmaxDatabasePassword,
		EnvKeyBuildmaxTestDSN,
		// The run token is read by a worker but never inherited: the scheduler
		// injects the one that names this run, so a stale value must not travel.
		EnvKeyBuildmaxRunToken,
	}
	for _, name := range withheld {
		if WorkerNeedsEnv(name, false) {
			t.Errorf("%s must not reach a worker", name)
		}
		if slices.Contains(WorkerEnvKeys(false), name) {
			t.Errorf("%s appears in WorkerEnvKeys", name)
		}
	}
}

// TestWorkerEnv_KeepsWhatTheWorkerReads guards the other direction: withholding
// something a worker needs breaks every run, so the two lists are asserted
// against each other rather than one being trusted.
func TestWorkerEnv_KeepsWhatTheWorkerReads(t *testing.T) {
	needed := []string{
		EnvKeyBuildmaxHome,
		EnvKeyBuildmaxServerURL,
		EnvKeyBuildmaxMinIOAccessKey,
		EnvKeyBuildmaxMinIOSecretKey,
		EnvKeyBuildmaxConversationAPIKey,
		EnvKeyBuildmaxSandboxEnabled,
		EnvKeyBuildmaxTraceDisabled,
	}
	for _, name := range needed {
		if !WorkerNeedsEnv(name, false) {
			t.Errorf("%s is read by a worker but withheld from it", name)
		}
	}
	if got, want := len(WorkerEnvKeys(false)), len(needed); got != want {
		t.Errorf("WorkerEnvKeys has %d entries, want %d — a new worker variable needs a decision here, not a default",
			got, want)
	}
}

// TestWorkerNeedsEnv_UnknownVariableIsWithheld asserts the default direction. A
// variable added to the server without a thought for workers stays on the
// server.
func TestWorkerNeedsEnv_UnknownVariableIsWithheld(t *testing.T) {
	if WorkerNeedsEnv("BUILDMAX_SOMETHING_ADDED_LATER", false) {
		t.Error("an unrecognized variable must default to withheld")
	}
}

func TestFilterWorkerEnv(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"HOME=/root",
		EnvKeyBuildmaxHome + "=/run/home",
		EnvKeyBuildmaxServerURL + "=https://server.example",
		EnvKeyBuildmaxMinIOAccessKey + "=minio-key",
		EnvKeyBuildmaxRunToken + "=some-other-runs-token",
		EnvKeyBuildmaxJWTSecret + "=jwt-secret",
		EnvKeyBuildmaxDatabasePassword + "=db-secret",
		"BUILDMAX_UNKNOWN=whatever",
		"MALFORMED_NO_EQUALS",
	}
	got := FilterWorkerEnv(in, false)
	joined := strings.Join(got, "\n")

	for _, banned := range []string{"jwt-secret", "db-secret", "BUILDMAX_UNKNOWN", "some-other-runs-token"} {
		if strings.Contains(joined, banned) {
			t.Errorf("filtered environment still carries %q: %v", banned, got)
		}
	}
	// Non-BUILDMAX variables are what let the binary run at all.
	for _, kept := range []string{"PATH=/usr/bin", "HOME=/root", EnvKeyBuildmaxHome + "=/run/home", EnvKeyBuildmaxServerURL + "=https://server.example", EnvKeyBuildmaxMinIOAccessKey + "=minio-key"} {
		if !slices.Contains(got, kept) {
			t.Errorf("filtered environment dropped %q: %v", kept, got)
		}
	}
	// A malformed entry has no name to judge, so it is left alone rather than
	// silently swallowed.
	if !slices.Contains(got, "MALFORMED_NO_EQUALS") {
		t.Errorf("malformed entry was dropped: %v", got)
	}
}

// TestManagedRunsGetNoProviderKey is the credential this whole change exists to
// remove. A managed run reaches models through the server, so the upstream
// provider key has no use inside a process that executes model-chosen shell
// commands — and a key with no use is pure exposure.
func TestManagedRunsGetNoProviderKey(t *testing.T) {
	if WorkerNeedsEnv(EnvKeyBuildmaxConversationAPIKey, true) {
		t.Errorf("%s reached a managed worker", EnvKeyBuildmaxConversationAPIKey)
	}
	if slices.Contains(WorkerEnvKeys(true), EnvKeyBuildmaxConversationAPIKey) {
		t.Errorf("%s appears in a managed worker's keys", EnvKeyBuildmaxConversationAPIKey)
	}

	filtered := FilterWorkerEnv([]string{
		EnvKeyBuildmaxConversationAPIKey + "=provider-key",
		EnvKeyBuildmaxMinIOAccessKey + "=minio-key",
	}, true)
	joined := strings.Join(filtered, "\n")
	if strings.Contains(joined, "provider-key") {
		t.Errorf("a managed worker was handed the provider key: %v", filtered)
	}
	// Everything else a worker reads is unaffected: managed inference changes
	// where prompts go, not how a run reports results or writes artifacts.
	for _, kept := range []string{"minio-key"} {
		if !strings.Contains(joined, kept) {
			t.Errorf("managed filtering dropped %q: %v", kept, filtered)
		}
	}
}

// TestDirectRunsStillGetTheProviderKey is the other direction. Direct mode is a
// first-class path, and withholding its credential would break every existing
// deployment.
func TestDirectRunsStillGetTheProviderKey(t *testing.T) {
	if !WorkerNeedsEnv(EnvKeyBuildmaxConversationAPIKey, false) {
		t.Errorf("%s was withheld from a direct worker", EnvKeyBuildmaxConversationAPIKey)
	}
	if got, want := len(WorkerEnvKeys(true)), len(WorkerEnvKeys(false))-1; got != want {
		t.Errorf("a managed worker gets %d variables, want %d — exactly one fewer than a direct one", got, want)
	}
}
