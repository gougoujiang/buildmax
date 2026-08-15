package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/infra/git"
	"github.com/gougoujiang/buildmax/internal/infra/sandbox"

	"github.com/spf13/cobra"
)

type doctorSeverity int

const (
	doctorOK doctorSeverity = iota
	doctorWarn
	doctorFail
)

type doctorCheck struct {
	Severity doctorSeverity
	Title    string
	Detail   string
	Next     string
}

func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check local setup before the first run",
		Long: `Check the local BuildMax setup without contacting an LLM provider.

Doctor verifies the app data directory, settings.yaml, model entries, workspace
state, git availability, and sandbox dependencies. It exits 2 only when a
required first-run prerequisite is missing.`,
		Args:         cobra.NoArgs,
		RunE:         runDoctor,
		SilenceUsage: true,
	}
	cmd.Flags().String("workspace", "", "workspace directory to check (default: current directory)")
	return cmd
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	workspace, _ := cmd.Flags().GetString("workspace")
	checks := collectDoctorChecks(workspace)
	writeDoctorReport(cmd.OutOrStdout(), checks)
	if doctorHasFailure(checks) {
		return &ExitError{Code: ExitUsage, Err: errors.New("doctor found setup problems")}
	}
	return nil
}

func collectDoctorChecks(workspace string) []doctorCheck {
	checks := []doctorCheck{
		checkDataDir(),
		checkGitAvailable(),
	}
	settings, settingsChecks := checkSettings()
	checks = append(checks, settingsChecks...)
	checks = append(checks, checkWorkspace(workspace))
	checks = append(checks, checkSandboxDeps(settings))
	return checks
}

func checkDataDir() doctorCheck {
	dir := config.DataDir()
	if dir == "" {
		return doctorCheck{Severity: doctorFail, Title: "BUILDMAX_HOME", Detail: "app data directory resolved to an empty path"}
	}
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			return doctorCheck{
				Severity: doctorFail,
				Title:    "BUILDMAX_HOME",
				Detail:   fmt.Sprintf("%s exists but is not a directory", dir),
				Next:     "Set BUILDMAX_HOME to a directory, or remove the file at that path.",
			}
		}
		return doctorCheck{Severity: doctorOK, Title: "BUILDMAX_HOME", Detail: dir}
	} else if errors.Is(err, fs.ErrNotExist) {
		return doctorCheck{
			Severity: doctorWarn,
			Title:    "BUILDMAX_HOME",
			Detail:   fmt.Sprintf("%s does not exist yet", dir),
			Next:     "Run `buildmax init` to create it with a starter settings.yaml.",
		}
	} else {
		return doctorCheck{Severity: doctorFail, Title: "BUILDMAX_HOME", Detail: fmt.Sprintf("cannot inspect %s: %v", dir, err)}
	}
}

func checkSettings() (config.Settings, []doctorCheck) {
	path := config.SettingsPath()
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return config.Settings{}, []doctorCheck{{
				Severity: doctorFail,
				Title:    "settings.yaml",
				Detail:   fmt.Sprintf("%s was not found", path),
				Next:     "Run `buildmax init --api-key <key>` or `buildmax init` to create it.",
			}}
		}
		return config.Settings{}, []doctorCheck{{
			Severity: doctorFail,
			Title:    "settings.yaml",
			Detail:   fmt.Sprintf("cannot inspect %s: %v", path, err),
		}}
	}

	settings, err := config.LoadSettings()
	if err != nil {
		return config.Settings{}, []doctorCheck{{
			Severity: doctorFail,
			Title:    "settings.yaml",
			Detail:   err.Error(),
			Next:     "Fix the YAML syntax, or re-run `buildmax init --force` to regenerate a starter file.",
		}}
	}

	checks := []doctorCheck{{
		Severity: doctorOK,
		Title:    "settings.yaml",
		Detail:   path,
	}}
	checks = append(checks, checkModels(settings)...)
	return settings, checks
}

func checkModels(settings config.Settings) []doctorCheck {
	if len(settings.Models) == 0 {
		return []doctorCheck{{
			Severity: doctorFail,
			Title:    "models",
			Detail:   "no models are configured",
			Next:     "Add a models: entry, or run `buildmax init --force` to regenerate settings.yaml.",
		}}
	}

	checks := make([]doctorCheck, 0, len(settings.Models))
	for i, m := range settings.Models {
		title := fmt.Sprintf("model[%d]", i)
		sev := optionalModelSeverity(i)
		display := m.Name
		if display == "" {
			display = m.Model
		}
		if strings.TrimSpace(m.Model) == "" {
			checks = append(checks, doctorCheck{
				Severity: sev,
				Title:    title,
				Detail:   "model id is empty",
				Next:     "Set the model field, for example `openai/gpt-4o-mini`.",
			})
			continue
		}
		if strings.TrimSpace(m.APIKey) == "" || m.APIKey == APIKeyPlaceholder {
			checks = append(checks, doctorCheck{
				Severity: sev,
				Title:    title,
				Detail:   fmt.Sprintf("%s has no real api_key", display),
				Next:     fmt.Sprintf("Replace %s on the api_key line with a real key.", APIKeyPlaceholder),
			})
			continue
		}
		if strings.TrimSpace(m.APIURL) == "" {
			checks = append(checks, doctorCheck{
				Severity: sev,
				Title:    title,
				Detail:   fmt.Sprintf("%s has an empty api_url", display),
				Next:     fmt.Sprintf("Set api_url, for example %s.", config.DefaultOpenRouterBaseURL),
			})
			continue
		}
		suffix := ""
		if i == 0 {
			suffix = " (default)"
		}
		checks = append(checks, doctorCheck{
			Severity: doctorOK,
			Title:    title,
			Detail:   fmt.Sprintf("%s -> %s%s", display, m.APIURL, suffix),
		})
	}
	return checks
}

func optionalModelSeverity(index int) doctorSeverity {
	if index == 0 {
		return doctorFail
	}
	return doctorWarn
}

func checkWorkspace(workspace string) doctorCheck {
	dir := workspace
	if strings.TrimSpace(dir) == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return doctorCheck{Severity: doctorFail, Title: "workspace", Detail: fmt.Sprintf("cannot read current directory: %v", err)}
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return doctorCheck{Severity: doctorFail, Title: "workspace", Detail: fmt.Sprintf("cannot resolve %s: %v", dir, err)}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return doctorCheck{Severity: doctorFail, Title: "workspace", Detail: fmt.Sprintf("cannot inspect %s: %v", abs, err)}
	}
	if !info.IsDir() {
		return doctorCheck{
			Severity: doctorFail,
			Title:    "workspace",
			Detail:   fmt.Sprintf("%s is not a directory", abs),
			Next:     "Pass a directory with `--workspace <dir>`.",
		}
	}
	branch := git.CurrentBranch(abs)
	if branch == "" {
		return doctorCheck{
			Severity: doctorWarn,
			Title:    "workspace",
			Detail:   fmt.Sprintf("%s is not on a named git branch", abs),
			Next:     "First runs are safest in a git working tree you can diff and revert.",
		}
	}
	return doctorCheck{Severity: doctorOK, Title: "workspace", Detail: fmt.Sprintf("%s (git branch %s)", abs, branch)}
}

func checkGitAvailable() doctorCheck {
	path, err := exec.LookPath("git")
	if err != nil {
		return doctorCheck{
			Severity: doctorWarn,
			Title:    "git",
			Detail:   "git was not found on PATH",
			Next:     "Install git so you can inspect and revert agent changes.",
		}
	}
	return doctorCheck{Severity: doctorOK, Title: "git", Detail: path}
}

func checkSandboxDeps(settings config.Settings) doctorCheck {
	if !settings.Sandbox.Enabled {
		return doctorCheck{
			Severity: doctorWarn,
			Title:    "sandbox",
			Detail:   "disabled for local runs",
			Next:     "Optional: run `buildmax sandbox enable` and then `buildmax sandbox deps`.",
		}
	}
	rep := sandbox.CheckDeps()
	if rep.AllRequiredOK() {
		return doctorCheck{Severity: doctorOK, Title: "sandbox", Detail: fmt.Sprintf("%s backend dependencies are present", rep.Backend)}
	}
	missing := rep.FirstMissingRequired()
	next := missing.Hint
	if next == "" {
		next = "Run `buildmax sandbox deps` for platform-specific details."
	}
	return doctorCheck{
		Severity: doctorWarn,
		Title:    "sandbox",
		Detail:   fmt.Sprintf("enabled, but required dependency %s is missing", missing.Name),
		Next:     next,
	}
}

func writeDoctorReport(w io.Writer, checks []doctorCheck) {
	fmt.Fprintln(w, "BuildMax doctor")
	fmt.Fprintln(w, "===============")
	for _, c := range checks {
		fmt.Fprintf(w, "%-6s %s: %s\n", doctorMarker(c.Severity), c.Title, c.Detail)
		if c.Next != "" {
			fmt.Fprintf(w, "       next: %s\n", c.Next)
		}
	}
	failures, warnings := doctorCounts(checks)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Summary: %d failure(s), %d warning(s)\n", failures, warnings)
	if failures == 0 {
		fmt.Fprintln(w, "Ready for a first run:")
		fmt.Fprintln(w, "  buildmax -p \"Summarize what this project does\"")
	}
}

func doctorMarker(s doctorSeverity) string {
	switch s {
	case doctorFail:
		return "[FAIL]"
	case doctorWarn:
		return "[WARN]"
	default:
		return "[OK]"
	}
}

func doctorCounts(checks []doctorCheck) (failures, warnings int) {
	for _, c := range checks {
		switch c.Severity {
		case doctorFail:
			failures++
		case doctorWarn:
			warnings++
		}
	}
	return failures, warnings
}

func doctorHasFailure(checks []doctorCheck) bool {
	failures, _ := doctorCounts(checks)
	return failures > 0
}
