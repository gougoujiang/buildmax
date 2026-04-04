package cli

import (
	"fmt"
	"log/slog"

	"buildmax/internal/auth"
	"buildmax/internal/config"
	"buildmax/internal/tui"
	"buildmax/internal/util"

	tea "github.com/charmbracelet/bubbletea"
)

// runTUI builds agent and session via setupAgentAndSession, then runs the TUI.
func runTUI(resumeID string, modelSelector string) error {
	res, err := setupAgentAndSession(resumeID, modelSelector)
	if err != nil {
		return err
	}
	defer res.Runtime.Close()
	var userEmail string
	if creds, err := auth.Load(config.AuthPath()); err == nil && creds != nil {
		userEmail = creds.Email
	}
	opts := tui.TUIOpts{
		Agent:       res.Runtime.Agent,
		LLMClient:   res.Runtime.LLMClient,
		Session:     res.Runtime.Session,
		ModelName:   res.ModelName,
		Workspace:   res.CWD,
		Branch:      util.CurrentBranch(res.CWD),
		Version:     Version,
		SessionsDir: res.SessionsDir,
		UserEmail:   userEmail,
	}
	p := tea.NewProgram(tui.NewModel(opts), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		slog.Error("TUI failed", "err", err)
		return fmt.Errorf("TUI: %w", err)
	}
	return nil
}
