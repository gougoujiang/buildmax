package cli

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/interface/auth"
	"github.com/muesli/termenv"
)

// detectGlamourStyle returns "dark" or "light" by querying the terminal background
// color. This must be called BEFORE tea.NewProgram so the terminal response arrives
// on stdin before Bubble Tea takes ownership of it.
func detectGlamourStyle() string {
	if termenv.HasDarkBackground() {
		return "dark"
	}
	return "light"
}

func runTUI(resumeID, modelName, additionalSystemPrompt string) error {
	app, err := agentapp.NewAgentApp(agentapp.AppConfig{
		EnableMCP:              true,
		Policy:                 agentapp.NewInteractivePolicy(),
		ManagedToken:           auth.TokenForServer,
		Surface:                model.LLMCallSurfaceCLI,
		AdditionalSystemPrompt: additionalSystemPrompt,
	})
	if err != nil {
		return err
	}
	defer app.Close()
	sess, err := app.OpenSession(resumeID)
	if err != nil {
		return err
	}
	if modelName != "" {
		sess.SetModel(modelName)
	}
	runStatus, err := app.EstimateRunStatus(sess)
	if err != nil {
		runStatus = agentapp.RunStatus{}
	}

	// Detect terminal color scheme before the program starts so glamour never
	// needs to query the terminal (which would inject escape-sequence responses
	// into stdin and corrupt the input box).
	glamourStyle := detectGlamourStyle()

	// Print banner and any existing session history to the terminal scrollback
	// before the TUI program takes over the bottom strip.
	fmt.Print(buildHistoryForScrollback(sess.Session, 80, glamourStyle))

	approval := NewTUIApprovalHandler()
	opts := TUIOpts{
		App:          app,
		Session:      sess,
		ModelName:    sess.ModelName(app.DefaultModelName()),
		Workspace:    app.WorkspaceRoot(),
		SessionsDir:  app.SessionsDir(),
		Approval:     approval,
		GlamourStyle: glamourStyle,
		RunStatus:    runStatus,
	}
	p := tea.NewProgram(NewModel(opts))
	approval.SetProgram(p)
	if _, err := p.Run(); err != nil {
		slog.Error("TUI failed", "err", err)
		return fmt.Errorf("TUI: %w", err)
	}
	return nil
}
