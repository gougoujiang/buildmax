package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/session"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// manyModelsSettings is a settings file with more models than any panel budget,
// so the /model panel has to trim rather than print them all.
func manyModelsSettings(n int) string {
	entries := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		entries = append(entries, fmt.Sprintf(
			`{"model":"openai/model-%02d","name":"Model %02d","api_url":"https://api.example.com","api_key":"sk-test"}`, i, i))
	}
	return `{"models":[` + strings.Join(entries, ",") + `]}`
}

// The TUI renders inline: whatever the view is taller than the terminal falls
// off the top, taking the input and the footer with it, and no key can scroll
// it back. Every panel has to fit what it lists into the height it was given.
func TestSlashPanelsFitTerminalHeight(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	writeTestSettings(t, manyModelsSettings(20))

	sessionsDir := t.TempDir()
	for i := range 20 {
		item := session.SessionItem{
			ID:        fmt.Sprintf("sess-%02d", i),
			Title:     fmt.Sprintf("chat number %02d", i),
			CreatedAt: fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1),
		}
		if err := agentapp.UpsertSessionItem(sessionsDir, item); err != nil {
			t.Fatalf("UpsertSessionItem: %v", err)
		}
	}

	app := testAgentAppWithJobs(t, home)
	for i := range 6 {
		if _, err := app.Jobs().StartCommand(testShellJobSpec("sleep 30"), job.Provenance{SessionID: fmt.Sprintf("s%d", i)}); err != nil {
			t.Fatalf("StartCommand: %v", err)
		}
	}

	for _, height := range []int{16, 20, 24, 30, 40} {
		for _, cmd := range []string{"/model", "/tools", "/skills", "/mcp", "/sessions", "/tasks", "/diff"} {
			t.Run(fmt.Sprintf("%s_h%d", strings.TrimPrefix(cmd, "/"), height), func(t *testing.T) {
				m := NewModel(TUIOpts{
					App:         app,
					Session:     testSessionContext(),
					ModelName:   "Model 01",
					Workspace:   home,
					SessionsDir: sessionsDir,
				})
				next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: height})
				mod := next.(*Model)
				opened, _ := dispatchSlashCommand(mod, cmd)
				mod = opened.(*Model)
				if mod.activePanel == nil {
					t.Fatalf("%s did not open a panel", cmd)
				}
				if got := lipgloss.Height(mod.View().Content); got > height {
					t.Errorf("%s view is %d lines on a %d-line terminal:\n%s", cmd, got, height, mod.View().Content)
				}
			})
		}
	}
}

// The completion popup is rebuilt on every message, not only on key presses, so
// a dismissal that is not remembered is undone by the next cursor blink.
func TestSlashPopupStaysDismissedUntilInputChanges(t *testing.T) {
	m := NewModel(TUIOpts{Session: testSessionContext(), Workspace: t.TempDir()})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	mod := next.(*Model)

	mod.inputBlock.SetValue("/")
	mod.syncSlashPopupFromInput()
	if mod.slashPopup == nil {
		t.Fatal("typing / should open the completion popup")
	}

	dismissed, _ := mod.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	mod = dismissed.(*Model)
	if mod.slashPopup != nil {
		t.Fatal("esc should dismiss the popup")
	}

	// What a blink tick does: rebuild the popup from unchanged input.
	mod.syncSlashPopupFromInput()
	if mod.slashPopup != nil {
		t.Fatal("popup should stay dismissed while the input is unchanged")
	}

	mod.inputBlock.SetValue("/mo")
	mod.syncSlashPopupFromInput()
	if mod.slashPopup == nil {
		t.Fatal("typing after a dismissal should open the popup again")
	}
}
