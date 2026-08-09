// Package taskrun prepares and executes task runs.
//
// agents_md.go: prepare AGENTS.md in the run directory so the in-process agent
// gets run context (directory layout + optional workspace AGENTS.md) via ReadAgentsMd.

package taskrun

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gougoujiang/buildmax/internal/agentapp"
)

// runDirectoryStructureSection is the generated Markdown describing the run directory layout.
// It is written into runDir/AGENTS.md so the agent receives it via the CLI's existing AGENTS.md path.
const runDirectoryStructureSection = `## Run directory layout

This run uses the following layout (relative to current working directory):

- **home/** — Working copy of workspace files. Read files here.
- **artifacts/** — Run output directory. Write results here (e.g. ` + "`result.md`" + `). Contents are archived when the run finishes. You can create new files here.
- **global/** — System settings for this run (sessions, logs, settings). Do not rely on paths inside global for tool outputs, and you should NEVER NEVER write to this directory.`

// BuildRunAgentsMdContent builds the full AGENTS.md content for a run: the run directory structure
// section first, then optional workspace AGENTS.md from runHome (if present).
// runHome is the materialized workspace home dir (run's home/).
func BuildRunAgentsMdContent(runHome string) (string, error) {
	out := strings.TrimSpace(runDirectoryStructureSection)
	extra, err := agentapp.ReadAgentsMd(runHome)
	if err != nil {
		slog.Warn("runtime: read workspace AGENTS.md for run", "run_home", runHome, "err", err)
		// Continue with structure only
		return out, nil
	}
	if extra != "" {
		out = out + "\n\n" + extra
	}
	return out, nil
}

// WriteRunAgentsMd writes AGENTS.md into runDir with content from BuildRunAgentsMdContent(runHome).
// Call after materializing runHome and before running the agent so it picks up run context from cwd.
func WriteRunAgentsMd(runDir, runHome string) error {
	content, err := BuildRunAgentsMdContent(runHome)
	if err != nil {
		return err
	}
	path := filepath.Join(runDir, agentapp.AgentsMdFilename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write run AGENTS.md: %w", err)
	}
	return nil
}
