package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/icloudbb/buildmax/internal/config"
)

// TestAdminListRequiresSignIn: the admin commands are authenticated API calls,
// so with no signed-in session they name the fix rather than failing obscurely.
func TestAdminListRequiresSignIn(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxHome, t.TempDir())
	cmd := newAdminListCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("err = %v, want a not-signed-in error", err)
	}
}

// TestAdminCommandHasItsVerbs pins the command surface so a lost registration is
// a failed test rather than a missing feature discovered at the terminal.
func TestAdminCommandHasItsVerbs(t *testing.T) {
	want := map[string]bool{"list": false, "grant": false, "revoke": false}
	for _, sub := range newAdminCommand().Commands() {
		want[sub.Name()] = true
	}
	for verb, found := range want {
		if !found {
			t.Errorf("admin is missing the %q subcommand", verb)
		}
	}
}
