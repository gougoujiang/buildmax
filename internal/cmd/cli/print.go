package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"buildmax/internal/agent"
	"buildmax/internal/config"
	"buildmax/internal/executor"
	"buildmax/internal/llm"
	"buildmax/internal/session"
)

// stdoutStreamSink writes each delta to stdout and flushes so output appears incrementally.
type stdoutStreamSink struct {
	w *os.File
}

func (s *stdoutStreamSink) OnDelta(delta string) {
	_, _ = s.w.WriteString(delta)
	_ = s.w.Sync()
}

func runPrintMode(prompt string, resumeID string, modelSelector string) error {
	res, err := setupAgentAndSession(resumeID, modelSelector)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return err
	}
	ctx := context.Background()
	start := time.Now()
	sink := &stdoutStreamSink{w: os.Stdout}
	reply, stats, err := res.Agent.Process(ctx, res.Session, prompt, agent.WithStreamSink(sink))
	elapsed := time.Since(start)
	slog.Debug("session details", "id", res.Session.ID(), "title", res.Session.Title(), "created_at", res.Session.CreatedAt())
	slog.Debug("session history", "messages", res.Session.Messages())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return fmt.Errorf("agent: %w", err)
	}
	// Generate an LLM title for new sessions (no title yet).
	if res.Session.Title() == "" {
		titleClient := session.TitleChatFunc(func(ctx context.Context, msgs []llm.Message) (string, llm.Usage, error) {
			content, _, usage, err := res.LLMClient.ChatWithTools(ctx, msgs, nil)
			return content, usage, err
		})
		title, titleUsage, err := session.GenerateTitle(ctx, titleClient, res.Session.Messages())
		if err != nil {
			slog.Warn("LLM title generation failed, using fallback", "err", err)
		} else {
			if titleUsage.PromptTokens > 0 || titleUsage.CompletionTokens > 0 {
				res.Session.AddUsage(titleUsage.PromptTokens, titleUsage.CompletionTokens)
			}
			if title != "" {
				res.Session.SetTitle(title)
			}
		}
	}
	res.Session.AddUsage(stats.PromptTokens, stats.CompletionTokens)
	if err := session.PersistAfterReply(res.Session, res.SessionsDir, res.CWD, 100); err != nil {
		slog.Error("persist session failed", "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return fmt.Errorf("persist session: %w", err)
	}
	if res.Session.PromptTokens() > 0 || res.Session.CompletionTokens() > 0 {
		if writeErr := executor.WriteUsageFile(config.DataDir(), res.Session.PromptTokens(), res.Session.CompletionTokens()); writeErr != nil {
			slog.Warn("write usage file failed", "err", writeErr)
		}
	}
	slog.Info("agent reply", "len", len(reply))
	// Reply was already streamed to stdout; print newline before stats if we streamed something.
	if len(reply) > 0 {
		fmt.Fprintln(os.Stdout)
	}

	// Print run statistics.
	fmt.Fprintln(os.Stdout, "---")
	fmt.Fprintf(os.Stdout, "Session:    %s\n", res.Session.ID())
	fmt.Fprintf(os.Stdout, "Tool calls: %d\n", stats.ToolCalls)
	fmt.Fprintf(os.Stdout, "Duration:   %s\n", formatDuration(elapsed))
	fmt.Fprintf(os.Stdout, "Workspace:  %s\n", res.CWD)
	if res.Session.PromptTokens() > 0 || res.Session.CompletionTokens() > 0 {
		fmt.Fprintf(os.Stdout, "Usage:      %d prompt + %d completion tokens\n", res.Session.PromptTokens(), res.Session.CompletionTokens())
	}
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
