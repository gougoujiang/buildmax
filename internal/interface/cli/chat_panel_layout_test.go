package cli

import (
	"fmt"
	"github.com/gougoujiang/buildmax/internal/util"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/config"

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

	sessionsDir := filepath.Join(home, "sessions")
	manager := agentapp.NewSessionManager(sessionsDir)
	for i := range 20 {
		id := fmt.Sprintf("sess-%02d", i)
		sess, err := manager.CreateWithID(id, "test-model")
		if err != nil {
			t.Fatalf("CreateWithID: %v", err)
		}
		// Closed as each one is made: the panel under test lists sessions, and
		// twenty held writer locks would be twenty open files for nothing.
		if err := sess.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := manager.Rename(id, fmt.Sprintf("chat number %02d", i)); err != nil {
			t.Fatalf("Rename: %v", err)
		}
	}

	app := testAgentAppWithJobs(t, home)
	for i := range 6 {
		if _, err := app.Jobs().StartCommand(testShellJobSpec("sleep 30"), job.Provenance{SessionID: fmt.Sprintf("s%d", i)}); err != nil {
			t.Fatalf("StartCommand: %v", err)
		}
	}

	for _, height := range []int{16, 20, 24, 30, 40} {
		for _, cmd := range []string{"/model", "/tools", "/skills", "/mcp", "/sessions", "/info", "/tasks", "/diff", "/worktree", "/agents", "/plugins"} {
			t.Run(fmt.Sprintf("%s_h%d", strings.TrimPrefix(cmd, "/"), height), func(t *testing.T) {
				m := NewModel(TUIOpts{
					App:         app,
					Session:     testSessionContext(),
					ModelName:   "Model 01",
					Workspace:   util.FixedRoot(home),
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
	m := NewModel(TUIOpts{Session: testSessionContext(), Workspace: util.FixedRoot(t.TempDir())})
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

// TestSlashModelPanelScrollsToTheSelection is the defect a trimmed list has
// that a short one does not: selection walks the whole list, so an entry past
// the fold could be switched to without ever being on screen — and the "… N
// more" row said so without offering any way to reach them.
func TestSlashModelPanelScrollsToTheSelection(t *testing.T) {
	const total = 20
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	writeTestSettings(t, manyModelsSettings(total))

	m := NewModel(TUIOpts{
		App:       testAgentApp(t, home),
		Session:   testSessionContext(),
		ModelName: "Model 01",
	})
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	opened, _ := dispatchSlashCommand(sized.(*Model), "/model")
	mod := opened.(*Model)
	if len(mod.slashModel.Entries) != total {
		t.Fatalf("entries = %d, want %d", len(mod.slashModel.Entries), total)
	}
	rows := mod.modelRowBudget(mod.slashModel)
	if rows >= total {
		t.Fatalf("row budget %d fits all %d models, so this test proves nothing", rows, total)
	}

	// Walk to the last entry. Every step must leave the selected row visible.
	for range total - 1 {
		next, _ := mod.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		mod = next.(*Model)
		content := mod.buildSlashModelContent(100)
		selected := mod.slashModel.Entries[mod.slashModel.Selected].Name
		if !strings.Contains(cursorRow(content), selected) {
			t.Fatalf("selection %q is off screen:\n%s", selected, content)
		}
	}
	if got := mod.slashModel.Selected; got != total-1 {
		t.Fatalf("selected = %d, want the last entry %d", got, total-1)
	}
	// At the bottom there is nothing below, so the panel says nothing about it.
	if strings.Contains(mod.buildSlashModelContent(100), "more") {
		t.Errorf("panel still claims entries below the last one:\n%s", mod.buildSlashModelContent(100))
	}

	// One more step wraps to the top, and the window has to come back with it.
	wrapped, _ := mod.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	mod = wrapped.(*Model)
	if mod.slashModel.Offset != 0 {
		t.Errorf("offset = %d, want 0 after wrapping to the first entry", mod.slashModel.Offset)
	}
}

// cursorRow returns the row the panel drew its selection cursor on, or "" when
// it drew none — which is the failure these tests are looking for.
func cursorRow(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "› ") {
			return line
		}
	}
	return ""
}

// TestSlashModelPanelOpensOnTheCurrentModel keeps the panel useful when the
// model in use is one of the ones past the fold: it opens showing the cursor,
// not the top of a list the cursor is not on.
func TestSlashModelPanelOpensOnTheCurrentModel(t *testing.T) {
	const total = 20
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	writeTestSettings(t, manyModelsSettings(total))

	m := NewModel(TUIOpts{
		App:       testAgentApp(t, home),
		Session:   testSessionContext(),
		ModelName: "Model 20",
	})
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	opened, _ := dispatchSlashCommand(sized.(*Model), "/model")
	mod := opened.(*Model)

	content := mod.buildSlashModelContent(100)
	if !strings.Contains(cursorRow(content), "Model 20") {
		t.Fatalf("panel should open on the model in use:\n%s", content)
	}
	if !strings.Contains(cursorRow(content), "*") {
		t.Errorf("the model in use should still be marked as current:\n%s", content)
	}
}

// TestSlashModelPanelSurvivesShrinking covers a panel that is open and scrolled
// when the terminal gets shorter: the window has to move back inside the list
// rather than leaving the bottom rows blank.
func TestSlashModelPanelSurvivesShrinking(t *testing.T) {
	const total = 20
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	writeTestSettings(t, manyModelsSettings(total))

	m := NewModel(TUIOpts{
		App:       testAgentApp(t, home),
		Session:   testSessionContext(),
		ModelName: "Model 20",
	})
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	opened, _ := dispatchSlashCommand(sized.(*Model), "/model")
	mod := opened.(*Model)

	shrunk, _ := mod.Update(tea.WindowSizeMsg{Width: 100, Height: 16})
	mod = shrunk.(*Model)
	content := mod.buildSlashModelContent(100)
	rows := mod.modelRowBudget(mod.slashModel)
	if mod.slashModel.Offset+rows > total {
		t.Errorf("window [%d,%d) runs past the %d entries", mod.slashModel.Offset, mod.slashModel.Offset+rows, total)
	}
	if !strings.Contains(content, "Model 20") {
		t.Errorf("the selected model should still be listed after the terminal shrank:\n%s", content)
	}
}

// TestSlashJobsPanelScrollsToTheSelection is the model panel's defect in the
// one place it can do damage: `s` stops the selected job, and a selection the
// panel never drew is a job the user cannot see being stopped.
func TestSlashJobsPanelScrollsToTheSelection(t *testing.T) {
	// Enough to overflow the row budget at this height and no more: every job
	// here is a real process, and Windows CI is slow to release the files one
	// leaves behind.
	const total = 8
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	writeTestSettings(t, manyModelsSettings(1))

	// The manager caps concurrent commands, and a panel long enough to scroll
	// is mostly finished jobs anyway — that is what a task list looks like
	// after a working session.
	app := testAgentAppWithJobs(t, home)
	for i := range total {
		// "exit 0" is the one immediate-success command both shells take: the
		// Windows spec wraps it in `cmd /c`, where `true` is not a command.
		started, err := app.Jobs().StartCommand(testShellJobSpec("exit 0"), job.Provenance{SessionID: fmt.Sprintf("s%d", i)})
		if err != nil {
			t.Fatalf("StartCommand: %v", err)
		}
		waitForJobToFinish(t, app, started.ID)
	}

	m := NewModel(TUIOpts{App: app, Session: testSessionContext(), Workspace: util.FixedRoot(home)})
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	opened, _ := dispatchSlashCommand(sized.(*Model), "/tasks")
	mod := opened.(*Model)
	panel, ok := mod.activePanel.(*slashJobsPanel)
	if !ok {
		t.Fatalf("active panel = %T, want *slashJobsPanel", mod.activePanel)
	}
	jobs := panel.jobs(mod)
	if rows := panel.rowBudget(mod); rows >= len(jobs) {
		t.Fatalf("row budget %d fits all %d jobs, so this test proves nothing", rows, len(jobs))
	}

	// Walk to the last job. Every step must leave the selected row drawn —
	// that row is what `s` acts on.
	for range len(jobs) - 1 {
		next, _ := mod.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		mod = next.(*Model)
		content := panel.Render(mod, 100)
		selected := panel.jobs(mod)[panel.selected].ID
		if !strings.Contains(jobsCursorRow(content), selected) {
			t.Fatalf("selection %q is off screen:\n%s", selected, content)
		}
	}
	if panel.selected != len(jobs)-1 {
		t.Fatalf("selected = %d, want the last job %d", panel.selected, len(jobs)-1)
	}
	if strings.Contains(panel.Render(mod, 100), "more") {
		t.Errorf("panel still claims jobs below the last one:\n%s", panel.Render(mod, 100))
	}

	// Selection does not wrap here, so the window must stay where it is.
	atEnd := panel.offset
	// The panel is shared with the returned model, so the assertions below read
	// the state this key press left behind.
	mod.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if panel.offset != atEnd || panel.selected != len(jobs)-1 {
		t.Errorf("down at the last job moved to offset %d selected %d, want %d and %d",
			panel.offset, panel.selected, atEnd, len(jobs)-1)
	}
}

// waitForJobToFinish keeps the manager's concurrency cap from rejecting the
// next start, and makes the list deterministic to render.
func waitForJobToFinish(t *testing.T, app *agentapp.AgentApp, id string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if snap, ok := app.Jobs().Get(id); ok && !snap.Running() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s never finished", id)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// jobsCursorRow returns the row the jobs panel drew its selection marker on.
func jobsCursorRow(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "> ") {
			return line
		}
	}
	return ""
}

// A ghost suggestion is rendered over the textarea's own height, so the input
// has to grow for one that wraps or the offer is shown cut in half.
func TestInputGrowsForAWrappingSuggestion(t *testing.T) {
	ib := NewInputBlock()
	ib.SetWidth(20)
	ib.SyncHeight()
	if got := ib.Height(); got != inputMinLines {
		t.Fatalf("empty input height = %d, want %d", got, inputMinLines)
	}

	ib.SetGhost("yes, and please also update the documentation for it")
	if got := ib.Height(); got <= inputMinLines {
		t.Errorf("height with a wrapping suggestion = %d, want more than %d", got, inputMinLines)
	}

	ib.SetGhost("")
	if got := ib.Height(); got != inputMinLines {
		t.Errorf("height after the suggestion is gone = %d, want %d", got, inputMinLines)
	}
}
