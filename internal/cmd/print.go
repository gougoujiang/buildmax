package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"buildmax/internal/session"
)

func runPrintMode(prompt string, resumeID string) error {
	res, err := setupAgentAndSession(resumeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return err
	}
	ctx := context.Background()
	reply, err := res.Agent.Process(ctx, res.Session, prompt)
	slog.Debug("session details", "id", res.Session.ID(), "title", res.Session.Title(), "created_at", res.Session.CreatedAt())
	slog.Debug("session history", "messages", res.Session.Messages())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return fmt.Errorf("agent: %w", err)
	}
	if err := session.PersistAfterReply(res.Session, res.SessionsDir, res.CWD, 100); err != nil {
		slog.Error("persist session failed", "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return fmt.Errorf("persist session: %w", err)
	}
	slog.Info("agent reply", "len", len(reply))
	fmt.Println(reply)
	return nil
}
