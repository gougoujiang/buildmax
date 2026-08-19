package cli

import (
	"strings"
	"testing"
)

func TestRootCommand_InvalidSessionIDReturnsError(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
	}{
		{"non-uuid string", "not-a-uuid"},
		{"short", "x"},
		{"too short", "123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := NewRootCommand()
			root.SetArgs([]string{"--session-id", tt.sessionID})
			err := root.Execute()
			if err == nil {
				t.Fatal("Execute(): want error for invalid --session-id")
			}
			if !strings.Contains(err.Error(), "invalid session-id") {
				t.Errorf("error message should contain 'invalid session-id': %q", err.Error())
			}
		})
	}
}

// TestRootCommand_FlagErrorPrecedesModelCheck pins the order of the two usage checks. A bad flag
// combination is fixable without a model configured, so reporting the missing configuration
// first would send the user to solve the wrong problem.
func TestRootCommand_FlagErrorPrecedesModelCheck(t *testing.T) {
	t.Setenv("BUILDMAX_HOME", t.TempDir()) // no settings.yaml, so the model check would also fail

	root := NewRootCommand()
	root.SetArgs([]string{
		"--append-system-prompt", "a",
		"--append-system-prompt-file", "b",
		"-p", "hi",
	})
	err := root.Execute()

	if err == nil {
		t.Fatal("Execute(): want an error for two mutually exclusive flags")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want the flag conflict rather than the model configuration", err.Error())
	}
}
