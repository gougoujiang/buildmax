package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	tools "github.com/gougoujiang/buildmax/internal/tool"
)

// systemPromptFlags holds the three ways the command line can add to a run's system prompt.
//
// What they produce is free text, appended as the prompt's last layer, so it survives compaction
// the way the runtime prompt does. A named definition is a convenience over the text, not a
// separate mechanism: its body fills the same slot.
type systemPromptFlags struct {
	Agent      string // --agent: a definition under .buildmax/agents or ~/.buildmax/agents
	AppendText string // --append-system-prompt
	AppendFile string // --append-system-prompt-file
}

// resolveAdditionalSystemPrompt turns the flags into the text appended to this run's system
// prompt.
//
// When both a named definition and ad-hoc text are given they are concatenated, definition
// first: a reusable base plus a customization for this run is a reasonable thing to ask for,
// and refusing it would buy nothing.
func resolveAdditionalSystemPrompt(f systemPromptFlags, workspace string) (string, error) {
	if f.AppendText != "" && f.AppendFile != "" {
		return "", fmt.Errorf("--append-system-prompt and --append-system-prompt-file are mutually exclusive")
	}

	var parts []string
	if f.Agent != "" {
		body, err := loadAgentPromptByName(f.Agent, workspace)
		if err != nil {
			return "", err
		}
		parts = append(parts, body)
	}
	switch {
	case f.AppendText != "":
		parts = append(parts, strings.TrimSpace(f.AppendText))
	case f.AppendFile != "":
		// A file rather than an argument because argv is world-readable through the process
		// list, and this is exactly the kind of text that carries something private. It also
		// spares the shell a multi-line quoting problem.
		b, err := os.ReadFile(f.AppendFile)
		if err != nil {
			return "", fmt.Errorf("read --append-system-prompt-file: %w", err)
		}
		if text := strings.TrimSpace(string(b)); text != "" {
			parts = append(parts, text)
		}
	}

	text := strings.Join(parts, "\n\n")
	if err := agentapp.ValidateAdditionalSystemPrompt(text); err != nil {
		return "", err
	}
	return text, nil
}

// loadAgentPromptByName resolves a definition from the same files the Task tool already loads:
// <workspace>/.buildmax/agents then ~/.buildmax/agents. No second format, no second search path.
// Only the body is used: --agent supplies prompt text, it does not switch model or tool set.
func loadAgentPromptByName(name, workspace string) (string, error) {
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve workspace for --agent: %w", err)
		}
		workspace = wd
	}
	// --agent resolves before a runtime exists, and a plugin-contributed agent
	// has to be reachable by name like any other.
	res, err := tools.ResolveAgentDefs(config.AgentDefSources(workspace, config.DiscoverPlugins().Loadable()))
	if err != nil {
		return "", fmt.Errorf("load agent definitions: %w", err)
	}
	var known []string
	for _, d := range res.Defs {
		if d.Name == name {
			if strings.TrimSpace(d.SystemPrompt) == "" {
				return "", fmt.Errorf("agent %q has an empty body, so it contributes no prompt text", name)
			}
			return strings.TrimSpace(d.SystemPrompt), nil
		}
		known = append(known, d.Name)
	}
	if len(known) == 0 {
		return "", fmt.Errorf("no agent named %q: no definitions found under %s",
			name, strings.Join(config.AgentDefsSearchPaths(workspace), " or "))
	}
	return "", fmt.Errorf("no agent named %q; available: %s", name, strings.Join(known, ", "))
}
