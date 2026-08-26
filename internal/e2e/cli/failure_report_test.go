package clie2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestAFailedRunStillReportsWhatItDid covers the run that fails after doing
// real work, which is the case a report is easiest to get wrong: the run read
// a file and spent tokens, and then the provider refused.
//
// Reporting none of that is not a cosmetic gap. "Tool calls: 0" on a run that
// edited the workspace sends someone looking in the wrong place, and tokens a
// provider charged for that no total mentions are tokens nobody can account
// for. The session's own metadata carries the same facts, because a session
// resumed after a failed turn otherwise counts from zero for good.
func TestAFailedRunStillReportsWhatItDid(t *testing.T) {
	server := startModel(t, "fails-after-a-tool-call.json")
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "alpha.txt"), []byte("the first file\n"), 0o600); err != nil {
		t.Fatalf("seed the workspace: %v", err)
	}
	home := writeHome(t, server, map[string]string{"Read": "allow"})

	result := run(t, home, workspace, "-p", "read then fail", "--output", "json")

	if result.exitCode != exitModelError {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", result.exitCode, exitModelError, result.stdout, result.stderr)
	}
	var envelope struct {
		SessionID  string `json:"session_id"`
		Workspace  string `json:"workspace"`
		Model      string `json:"model"`
		ToolCalls  int    `json:"tool_calls"`
		DurationMS int64  `json:"duration_ms"`
		TracePath  string `json:"trace_path"`
		Usage      struct {
			Prompt     int `json:"prompt"`
			Completion int `json:"completion"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &envelope); err != nil {
		t.Fatalf("decode the envelope: %v\nstdout:\n%s", err, result.stdout)
	}
	if envelope.Workspace == "" {
		t.Error("the failed run reports no workspace, though it ran in one and read from it")
	}
	if envelope.Model == "" {
		t.Error("the failed run reports no model, though the failure is about reaching one")
	}
	if envelope.ToolCalls != 1 {
		t.Errorf("tool_calls = %d, want 1: the read happened before the provider refused", envelope.ToolCalls)
	}
	if envelope.Usage.Prompt != 150 || envelope.Usage.Completion != 12 {
		t.Errorf("usage = %d/%d, want the scripted 150/12: tokens a provider charged for must be reported",
			envelope.Usage.Prompt, envelope.Usage.Completion)
	}
	if envelope.TracePath == "" {
		t.Error("the failed run names no trace, which is what a diagnosis starts from")
	}

	// The session carries it too. Reporting it only to stdout would leave the
	// conversation resumable and unaccounted: `-c` picks it up, and its totals
	// start from zero.
	meta := readSessionMeta(t, home, envelope.SessionID)
	if meta.Workspace == "" {
		t.Error("the session records no workspace after a failed turn")
	}
	if meta.PromptTokens != 150 || meta.CompletionTokens != 12 {
		t.Errorf("session totals = %d/%d, want the scripted 150/12", meta.PromptTokens, meta.CompletionTokens)
	}
}

type sessionMeta struct {
	Workspace        string `json:"workspace"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
}

// readSessionMeta reads one session bundle's metadata from disk. The suite goes
// to the file rather than through `buildmax stats` on purpose: what is being
// checked is that the fact was persisted, not that a command can format it.
func readSessionMeta(t *testing.T, home, sessionID string) sessionMeta {
	t.Helper()
	path := filepath.Join(home, "sessions", sessionID, "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var meta sessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("decode %s: %v\n%s", path, err, data)
	}
	return meta
}
