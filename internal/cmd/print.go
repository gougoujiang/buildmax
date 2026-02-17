package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"buildmax/internal/llm"
	"buildmax/internal/session"
)

func runPrintMode(prompt string, resumeID string, modelSelector string) error {
	res, err := setupAgentAndSession(resumeID, modelSelector)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return err
	}
	ctx := context.Background()
	start := time.Now()
	reply, stats, err := res.Agent.Process(ctx, res.Session, prompt)
	elapsed := time.Since(start)
	slog.Debug("session details", "id", res.Session.ID(), "title", res.Session.Title(), "created_at", res.Session.CreatedAt())
	slog.Debug("session history", "messages", res.Session.Messages())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return fmt.Errorf("agent: %w", err)
	}
	// Generate an LLM title for new sessions (no title yet).
	if res.Session.Title() == "" {
		titleClient := session.TitleChatFunc(func(ctx context.Context, msgs []llm.Message) (string, error) {
			content, _, err := res.LLMClient.ChatWithTools(ctx, msgs, nil)
			return content, err
		})
		title, err := session.GenerateTitle(ctx, titleClient, res.Session.Messages())
		if err != nil {
			slog.Warn("LLM title generation failed, using fallback", "err", err)
		} else if title != "" {
			res.Session.SetTitle(title)
		}
	}
	if err := session.PersistAfterReply(res.Session, res.SessionsDir, res.CWD, 100); err != nil {
		slog.Error("persist session failed", "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return fmt.Errorf("persist session: %w", err)
	}
	slog.Info("agent reply", "len", len(reply))
	fmt.Println(reply)

	// Print run statistics.
	fmt.Fprintf(os.Stdout, "\n---\nSession:    %s\nTool calls: %d\nDuration:   %s\nWorkspace:  %s\n", res.Session.ID(), stats.ToolCalls, formatDuration(elapsed), res.CWD)
	return nil
}

// formatDuration formats a duration in a human-friendly way (e.g. "1m23s", "4.5s", "120ms").
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}
