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
	}
	for _, name := range withheld {
		if WorkerNeedsEnv(name) {
			t.Errorf("%s must not reach a worker", name)
		}
		if slices.Contains(WorkerEnvKeys(), name) {
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
		EnvKeyBuildmaxWorkerToken,
		EnvKeyBuildmaxMinIOAccessKey,
		EnvKeyBuildmaxMinIOSecretKey,
		EnvKeyBuildmaxConversationAPIKey,
		EnvKeyBuildmaxSandboxEnabled,
		EnvKeyBuildmaxTraceDisabled,
	}
	for _, name := range needed {
		if !WorkerNeedsEnv(name) {
			t.Errorf("%s is read by a worker but withheld from it", name)
		}
	}
	if got, want := len(WorkerEnvKeys()), len(needed); got != want {
		t.Errorf("WorkerEnvKeys has %d entries, want %d — a new worker variable needs a decision here, not a default",
			got, want)
	}
}

// TestWorkerNeedsEnv_UnknownVariableIsWithheld asserts the default direction. A
// variable added to the server without a thought for workers stays on the
// server.
func TestWorkerNeedsEnv_UnknownVariableIsWithheld(t *testing.T) {
	if WorkerNeedsEnv("BUILDMAX_SOMETHING_ADDED_LATER") {
		t.Error("an unrecognized variable must default to withheld")
	}
}

func TestFilterWorkerEnv(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"HOME=/root",
		EnvKeyBuildmaxHome + "=/run/home",
		EnvKeyBuildmaxWorkerToken + "=wt-secret",
		EnvKeyBuildmaxJWTSecret + "=jwt-secret",
		EnvKeyBuildmaxDatabasePassword + "=db-secret",
		"BUILDMAX_UNKNOWN=whatever",
		"MALFORMED_NO_EQUALS",
	}
	got := FilterWorkerEnv(in)
	joined := strings.Join(got, "\n")

	for _, banned := range []string{"jwt-secret", "db-secret", "BUILDMAX_UNKNOWN"} {
		if strings.Contains(joined, banned) {
			t.Errorf("filtered environment still carries %q: %v", banned, got)
		}
	}
	// Non-BUILDMAX variables are what let the binary run at all.
	for _, kept := range []string{"PATH=/usr/bin", "HOME=/root", EnvKeyBuildmaxHome + "=/run/home", EnvKeyBuildmaxWorkerToken + "=wt-secret"} {
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
