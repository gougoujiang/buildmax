package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/interface/slashcmd"
	"github.com/gougoujiang/buildmax/internal/util"

	tea "charm.land/bubbletea/v2"
)

// The completion list must be exactly the CLI surface of the shared registry,
// so a command added there shows up in the TUI without a second edit.
func TestBuiltinSlashCommandsMatchRegistry(t *testing.T) {
	want := slashcmd.Names(slashcmd.CLI)
	if len(builtinSlashCommands) != len(want) {
		t.Fatalf("builtinSlashCommands = %v, want %v", builtinSlashCommands, want)
	}
	for i, name := range want {
		if builtinSlashCommands[i] != name {
			t.Fatalf("builtinSlashCommands[%d] = %q, want %q", i, builtinSlashCommands[i], name)
		}
	}
}

// Every command the registry offers on the CLI must have a dispatch case: a
// command in the completion list that falls through to "unknown command" is the
// drift this test exists to catch. It asserts dispatch reaches a handler, not
// what the handler does.
func TestEveryRegistryCommandDispatches(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	writeTestSettings(t, manyModelsSettings(3))
	sessionsDir := filepath.Join(home, "sessions")

	for _, c := range slashcmd.For(slashcmd.CLI) {
		t.Run(c.Name, func(t *testing.T) {
			m := NewModel(TUIOpts{
				App:         testAgentAppWithJobs(t, home),
				Session:     testSessionContext(),
				ModelName:   "Model 01",
				Workspace:   util.FixedRoot(home),
				SessionsDir: sessionsDir,
			})
			next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			mod := next.(*Model)
			out, _ := dispatchSlashCommand(mod, c.Slash())
			if got := out.(*Model).err; strings.HasPrefix(got, "unknown command") {
				t.Fatalf("%s fell through to: %s", c.Slash(), got)
			}
		})
	}
}
