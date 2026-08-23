package grader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

// FilesConfig asserts over the final workspace.
type FilesConfig struct {
	// Exists names paths that must be present after the run.
	Exists []string `json:"exists,omitempty"`
	// Absent names paths that must not be. Section 11 pairs a positive case
	// with a negative one: proving a file was written says nothing about
	// whether the run also left a mess behind.
	Absent []string `json:"absent,omitempty"`
	// Matches maps a path to a regular expression its content must satisfy.
	Matches map[string]string `json:"matches,omitempty"`
	// Equals maps a path to its exact expected content.
	Equals map[string]string `json:"equals,omitempty"`
}

// Files checks the final state of the workspace. It is the outcome-first
// grader of section 7.1: what the workspace holds, not what the reply claimed.
type Files struct{}

func (Files) Grade(_ context.Context, in Input) contract.GraderResult {
	var cfg FilesConfig
	if err := json.Unmarshal(nonEmpty(in.Ref.Config), &cfg); err != nil {
		return broken(fmt.Errorf("decode files config: %w", err))
	}

	var problems []string
	for _, rel := range cfg.Exists {
		path, err := insideWorkspace(in.Workspace, rel)
		if err != nil {
			return broken(err)
		}
		if _, err := os.Stat(path); err != nil {
			problems = append(problems, fmt.Sprintf("%s is missing", rel))
		}
	}
	for _, rel := range cfg.Absent {
		path, err := insideWorkspace(in.Workspace, rel)
		if err != nil {
			return broken(err)
		}
		if _, err := os.Stat(path); err == nil {
			problems = append(problems, fmt.Sprintf("%s should not exist", rel))
		}
	}

	for _, rel := range sortedKeys(cfg.Matches) {
		body, err := readInside(in.Workspace, rel)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", rel, err))
			continue
		}
		re, err := regexp.Compile(cfg.Matches[rel])
		if err != nil {
			// A malformed pattern is the task's defect, not the subject's.
			return broken(fmt.Errorf("compile pattern for %s: %w", rel, err))
		}
		if !re.Match(body) {
			problems = append(problems, fmt.Sprintf("%s does not match %s", rel, cfg.Matches[rel]))
		}
	}
	for _, rel := range sortedKeys(cfg.Equals) {
		body, err := readInside(in.Workspace, rel)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", rel, err))
			continue
		}
		if string(body) != cfg.Equals[rel] {
			problems = append(problems, fmt.Sprintf("%s content differs from the expected body", rel))
		}
	}

	if len(problems) > 0 {
		return failed(strings.Join(problems, "; "))
	}
	return passed("every file assertion held")
}

// CommandConfig runs a check the task ships.
type CommandConfig struct {
	// Run is the command, resolved inside the task's graders directory when
	// its first element is a relative path. The command runs with the
	// workspace as its working directory.
	Run []string `json:"run"`
	// TimeoutSeconds bounds the check. A grader that hangs would otherwise
	// consume the experiment rather than fail one trial.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	// ExpectExit is the exit code that means pass. Zero by default; a task
	// asserting that something correctly fails sets it.
	ExpectExit int `json:"expect_exit,omitempty"`
}

// Command runs a task-supplied check against the workspace: a test suite, a
// linter, a schema validation. The script lives in the task's graders
// directory, which the trial never saw.
type Command struct{}

const defaultCommandTimeout = 120

func (Command) Grade(ctx context.Context, in Input) contract.GraderResult {
	var cfg CommandConfig
	if err := json.Unmarshal(nonEmpty(in.Ref.Config), &cfg); err != nil {
		return broken(fmt.Errorf("decode command config: %w", err))
	}
	if len(cfg.Run) == 0 {
		return broken(fmt.Errorf("command grader has no command to run"))
	}

	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	name := cfg.Run[0]
	if strings.ContainsAny(name, "/\\") && !rooted(name) {
		// A relative path names something the task ships, so it resolves
		// against the graders directory rather than the workspace: resolving
		// it against the workspace would let the subject supply its own grader
		// by writing a file at the right path.
		//
		// It is then made absolute, because the command runs with the workspace
		// as its working directory. A task directory given relatively — which
		// is how a suite path arrives on the command line — would otherwise be
		// resolved against the workspace, the one place it must not come from.
		resolved, err := filepath.Abs(filepath.Join(in.TaskDir, contract.GradersDir, filepath.FromSlash(name)))
		if err != nil {
			return broken(fmt.Errorf("resolve grader command: %w", err))
		}
		name = resolved
	}

	cmd := exec.CommandContext(runCtx, name, cfg.Run[1:]...)
	cmd.Dir = in.Workspace
	cmd.WaitDelay = 10 * time.Second
	cmd.Env = append(os.Environ(),
		"BUILDMAX_TRIAL_WORKSPACE="+in.Workspace,
		"BUILDMAX_TRIAL_GRADERS="+filepath.Join(in.TaskDir, contract.GradersDir),
	)
	out, runErr := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if runCtx.Err() != nil {
		return broken(fmt.Errorf("grader command exceeded %ds: %s", timeout, bound(output, 2000)))
	}
	exit := cmd.ProcessState.ExitCode()
	if exit < 0 {
		// The command never produced an exit status — it could not start, or a
		// signal ended it. Either way nothing was measured.
		return broken(fmt.Errorf("grader command did not complete: %v: %s", runErr, bound(output, 2000)))
	}
	if exit != cfg.ExpectExit {
		return failed(fmt.Sprintf("exit %d, want %d: %s", exit, cfg.ExpectExit, bound(output, 2000)))
	}
	return passed(bound(output, 2000))
}

// insideWorkspace resolves a task-supplied relative path and refuses one that
// escapes. A task asserting on /etc/passwd is measuring the machine, not the
// subject.
func insideWorkspace(workspace, rel string) (string, error) {
	// A task file is portable, so its paths have to mean the same thing on every
	// host that runs the suite. filepath.IsAbs does not: "/etc/passwd" is
	// absolute on Unix and merely rooted on Windows, where Join would quietly
	// fold it into the workspace and the assertion would run instead of being
	// refused. Rejecting every rooted form, and any volume name, keeps one task
	// from meaning two things depending on where it ran.
	if rooted(rel) {
		return "", fmt.Errorf("path %q is absolute; assertions are workspace-relative", rel)
	}
	full := filepath.Join(workspace, filepath.FromSlash(rel))
	clean, err := filepath.Rel(workspace, full)
	if err != nil || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q leaves the workspace", rel)
	}
	return full, nil
}

// rooted reports whether a path is anchored somewhere other than the directory
// it is resolved against — on any host, not just this one.
//
// The standard library cannot answer this portably. filepath.IsAbs and
// VolumeName both describe the host they run on: `/etc/passwd` is absolute on
// Unix and merely rooted on Windows, and `C:\data` is a drive path on Windows
// and an ordinary relative name on Unix. Either gap lets one committed task
// mean two things depending on where the suite ran — refused on one platform,
// silently resolved inside the workspace on the other.
//
// It costs the ability to name a Unix file literally called "C:something",
// which is a trade worth making: a task that behaves differently per platform
// is worse than one that cannot name an implausible file.
func rooted(p string) bool {
	if p == "" {
		return false
	}
	if filepath.IsAbs(p) || p[0] == '/' || p[0] == '\\' {
		return true
	}
	// A Windows drive prefix, with or without a following separator: `C:\x` is
	// anchored to the drive root and `C:x` to that drive's current directory.
	// Neither is relative to the workspace.
	return len(p) >= 2 && p[1] == ':' &&
		((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z'))
}

func readInside(workspace, rel string) ([]byte, error) {
	path, err := insideWorkspace(workspace, rel)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("missing")
		}
		return nil, err
	}
	return body, nil
}

// nonEmpty lets a grader with no configuration decode as its zero value rather
// than as a JSON syntax error.
func nonEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func bound(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
