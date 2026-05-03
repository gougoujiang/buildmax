package cli

import (
	"context"
	"fmt"
	"os"

	"buildmax/internal/execution/agentrun"
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
	defer res.Runtime.Close()
	ctx := context.Background()
	sink := &stdoutStreamSink{w: os.Stdout}
	out, err := res.Runtime.RunPrompt(ctx, agentrun.RunInput{
		Prompt: prompt,
		Stream: sink,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return err
	}
	// Reply was already streamed to stdout; print newline before stats if we streamed something.
	if len(out.Reply) > 0 {
		fmt.Fprintln(os.Stdout)
	}

	// Print run statistics.
	fmt.Fprintln(os.Stdout, "---")
	fmt.Fprintf(os.Stdout, "Session:    %s\n", out.SessionID)
	fmt.Fprintf(os.Stdout, "Tool calls: %d\n", out.ToolCalls)
	fmt.Fprintf(os.Stdout, "Duration:   %s\n", agentrun.FormatDuration(out.Duration))
	fmt.Fprintf(os.Stdout, "Workspace:  %s\n", out.Workspace)
	if out.TotalPromptTokens > 0 || out.TotalCompletionTokens > 0 {
		fmt.Fprintf(os.Stdout, "Usage:      %d prompt + %d completion tokens\n", out.TotalPromptTokens, out.TotalCompletionTokens)
	}
	return nil
}
