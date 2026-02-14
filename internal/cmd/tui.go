package cmd

import (
	"fmt"
	"log/slog"

	"buildmax/internal/app"
	"buildmax/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

// runTUI builds agent and session via setupAgentAndSession, then runs the TUI.
func runTUI(resumeID string) error {
	res, err := setupAgentAndSession(resumeID)
	if err != nil {
		return err
	}
	opts := tui.TUIOpts{
		Agent:       res.Agent,
		LLMClient:   res.LLMClient,
		Session:     res.Session,
		ModelName:   res.ModelName,
		Workspace:   res.CWD,
		Version:     Version,
		SessionsDir: res.SessionsDir,
	}
	p := tea.NewProgram(app.NewModel(opts), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		slog.Error("TUI failed", "err", err)
		return fmt.Errorf("TUI: %w", err)
	}
	return nil
}
