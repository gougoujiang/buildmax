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
