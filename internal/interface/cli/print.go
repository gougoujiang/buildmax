package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/interface/auth"
	"github.com/gougoujiang/buildmax/internal/util"
)

// printOptions controls the behavior of runPrintMode. Built from CLI flags.
type printOptions struct {
	Prompt        string
	ResumeID      string
	ModelName     string
	Workspace     string
	Format        OutputFormat
	NoStream      bool
	Quiet         bool
	IncludeDeltas bool
	// AdditionalSystemPrompt is this run's user-authored prompt text, appended as the system
	// prompt's last layer. Empty leaves a resumed session running under whatever text it
	// already had.
	AdditionalSystemPrompt string
}

// stdoutStreamSink writes each delta to stdout and flushes so output appears incrementally.
type stdoutStreamSink struct {
	w io.Writer
}

func (s *stdoutStreamSink) OnDelta(delta string) {
	_, _ = io.WriteString(s.w, delta)
	if f, ok := s.w.(*os.File); ok {
		_ = f.Sync()
	}
}

// jsonlEventSink writes each (filtered) event as one JSON line to stdout.
type jsonlEventSink struct {
	w             io.Writer
	mu            sync.Mutex
	includeDeltas bool
}

func (s *jsonlEventSink) OnEvent(ev agent.Event) {
	line, ok := eventToJSON(ev, s.includeDeltas)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.w.Write(line)
	_, _ = io.WriteString(s.w, "\n")
}

func runPrintMode(opts printOptions) error {
	app, err := agentapp.NewAgentApp(agentapp.AppConfig{
		WorkspaceDir:           opts.Workspace,
		EnableMCP:              true,
		Policy:                 agentapp.NewNonInteractivePolicy(),
		ManagedToken:           auth.TokenForServer,
		Surface:                model.LLMCallSurfaceCLI,
		AdditionalSystemPrompt: opts.AdditionalSystemPrompt,
	})
	if err != nil {
		return printFatal(opts.Format, ExitModelError, err)
	}
	defer app.Close()
	sess, err := app.OpenSession(opts.ResumeID)
	if err != nil {
		return printFatal(opts.Format, ExitModelError, err)
	}
	if opts.ModelName != "" {
		sess.SetModel(opts.ModelName)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Choose stream sink: stream only in text mode, and only when --no-stream is not set.
	var streamSink cllm.StreamSink
	if opts.Format == OutputText && !opts.NoStream {
		streamSink = &stdoutStreamSink{w: os.Stdout}
	}

	// Track policy denials by observing events; also wire jsonl sink when needed.
	policyDenied := false
	eventSink := func(ev agent.Event) {
		if ev.Kind == agent.EventToolDenied && ev.DenyReason == agent.DenyReasonPolicy {
			policyDenied = true
		}
	}
	if opts.Format == OutputJSONL {
		jsonl := &jsonlEventSink{w: os.Stdout, includeDeltas: opts.IncludeDeltas}
		baseSink := eventSink
		eventSink = func(ev agent.Event) {
			baseSink(ev)
			jsonl.OnEvent(ev)
		}
	}

	out, runErr := app.RunPrompt(ctx, sess, opts.Prompt, agentapp.RunPromptOpts{Stream: streamSink, EventSink: eventSink})

	userCancelled := ctx.Err() != nil
	exitCode := classifyExit(runErr, policyDenied, userCancelled)

	switch opts.Format {
	case OutputJSON, OutputJSONL:
		emitResultEnvelope(os.Stdout, out, exitCode, runErr, policyDenied, opts.Format == OutputJSONL)
	default:
		emitTextSummary(os.Stdout, os.Stderr, out, runErr, opts)
	}

	if exitCode == ExitOK {
		return nil
	}
	return &ExitError{Code: exitCode, Err: runErr}
}

func emitResultEnvelope(w io.Writer, out agentapp.RunResult, exitCode int, runErr error, policyDenied bool, jsonl bool) {
	env := buildResultEnvelope(out, exitCode, runErr, policyDenied, jsonl)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(env)
}

func emitTextSummary(stdout, stderr io.Writer, out agentapp.RunResult, runErr error, opts printOptions) {
	// If we streamed, terminate the reply line. If we didn't stream but have a reply (--no-stream), print it now.
	if opts.NoStream && len(out.Reply) > 0 {
		fmt.Fprintln(stdout, out.Reply)
	} else if !opts.NoStream && len(out.Reply) > 0 {
		fmt.Fprintln(stdout)
	}

	if runErr != nil {
		fmt.Fprintf(stderr, "error: %v\n", runErr)
	}
	if opts.Quiet {
		return
	}
	fmt.Fprintln(stdout, "---")
	fmt.Fprintf(stdout, "Session:    %s\n", out.SessionID)
	fmt.Fprintf(stdout, "Tool calls: %d\n", out.ToolCalls)
	fmt.Fprintf(stdout, "Duration:   %s\n", util.FormatDuration(out.Duration))
	fmt.Fprintf(stdout, "Workspace:  %s\n", out.Workspace)
	if out.PromptTokens > 0 || out.CompletionTokens > 0 || out.TotalPromptTokens > 0 || out.TotalCompletionTokens > 0 {
		fmt.Fprintf(stdout, "Tokens(in/out): %s\n", formatTokenUsageValue(out.PromptTokens, out.CompletionTokens, out.TotalPromptTokens, out.TotalCompletionTokens))
	}
}

// printFatal handles errors that occur before the agent has run (setup
// failures): emit an envelope in machine-readable formats, stderr otherwise.
func printFatal(format OutputFormat, code int, err error) error {
	switch format {
	case OutputJSON, OutputJSONL:
		env := printResult{
			ExitCode: code,
			Error:    &printErrorObj{Kind: errorKindForExitCode(code), Message: err.Error()},
		}
		if format == OutputJSONL {
			env.Type = "result"
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(env)
	default:
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	return &ExitError{Code: code, Err: err}
}
