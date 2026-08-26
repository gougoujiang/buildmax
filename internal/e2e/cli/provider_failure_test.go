package clie2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// exitModelError mirrors cli.ExitModelError. It is repeated rather than
// imported because this suite's subject is the built binary's contract with a
// shell, and importing the constant would let a renumbering pass both sides.
const exitModelError = 4

// TestAProviderRefusalIsReportedUsefully covers the failure half of the CLI
// golden path. A run that cannot reach a model is the most common way this
// binary fails for a real person, and the three things they need from it are
// the exit code a script branches on, a message that names what to fix, and no
// silent retry storm against a credential that will never work.
//
// The scripted 401 is what a wrong api_key produces. It is deliberately not a
// 500: that one is retryable, and asserting on it would measure the backoff
// schedule rather than the failure path.
func TestAProviderRefusalIsReportedUsefully(t *testing.T) {
	server := startModel(t, "provider-refuses.json")
	home := writeHome(t, server, nil)

	result := run(t, home, t.TempDir(), "-p", "say hello")

	if result.exitCode != exitModelError {
		t.Fatalf("exit code = %d, want %d (model error)\nstderr:\n%s", result.exitCode, exitModelError, result.stderr)
	}
	// Both halves matter: the classification says what to do about it, and the
	// provider's own words say what actually came back.
	for _, want := range []string{"authentication failed (HTTP 401)", "api_key", "invalid api key"} {
		if !strings.Contains(result.stderr, want) {
			t.Errorf("stderr does not mention %q\nstderr:\n%s", want, result.stderr)
		}
	}
	// One call, not four. A permanent error retried is a user waiting out a
	// backoff to be told the same thing.
	if calls := len(server.Requests()); calls != 1 {
		t.Errorf("model calls = %d, want 1: a 401 is permanent and must not be retried", calls)
	}
	if remaining := server.Remaining(); remaining != 0 {
		t.Errorf("unconsumed scenario steps = %d, want 0", remaining)
	}
}

// TestAProviderRefusalIsMachineReadable is the same failure through --output
// json, which is what a script or another program reads. The exit code alone
// does not say which failure it was.
func TestAProviderRefusalIsMachineReadable(t *testing.T) {
	server := startModel(t, "provider-refuses.json")
	home := writeHome(t, server, nil)

	result := run(t, home, t.TempDir(), "-p", "say hello", "--output", "json")

	if result.exitCode != exitModelError {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s", result.exitCode, exitModelError, result.stdout)
	}
	var envelope struct {
		ExitCode int `json:"exit_code"`
		Error    *struct {
			Kind    string `json:"kind"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &envelope); err != nil {
		t.Fatalf("--output json did not produce one JSON document: %v\nstdout:\n%s", err, result.stdout)
	}
	if envelope.Error == nil {
		t.Fatalf("the envelope carries no error object\nstdout:\n%s", result.stdout)
	}
	if envelope.Error.Kind != "model_error" {
		t.Errorf("error kind = %q, want %q", envelope.Error.Kind, "model_error")
	}
	if envelope.ExitCode != exitModelError {
		t.Errorf("envelope exit_code = %d, want %d", envelope.ExitCode, exitModelError)
	}
	if !strings.Contains(envelope.Error.Message, "401") {
		t.Errorf("error message does not carry the status: %q", envelope.Error.Message)
	}
}
