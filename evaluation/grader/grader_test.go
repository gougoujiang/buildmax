package grader

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

func config(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return raw
}

func workspaceWith(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return dir
}

func TestFilesGrader(t *testing.T) {
	ws := workspaceWith(t, map[string]string{
		"report.md":    "# Report\n\nAll tests pass.\n",
		"src/main.go":  "package main\n",
		"leftover.tmp": "junk\n",
	})

	tests := []struct {
		name string
		cfg  FilesConfig
		want contract.Verdict
	}{
		{"present file", FilesConfig{Exists: []string{"report.md"}}, contract.VerdictPass},
		{"missing file", FilesConfig{Exists: []string{"absent.md"}}, contract.VerdictFail},
		{"absent holds", FilesConfig{Absent: []string{"nothing-here"}}, contract.VerdictPass},
		{"absent violated", FilesConfig{Absent: []string{"leftover.tmp"}}, contract.VerdictFail},
		{"pattern matches", FilesConfig{Matches: map[string]string{"report.md": `^# Report`}}, contract.VerdictPass},
		{"pattern misses", FilesConfig{Matches: map[string]string{"report.md": `^# Summary`}}, contract.VerdictFail},
		{"pattern on missing file", FilesConfig{Matches: map[string]string{"gone.md": `.`}}, contract.VerdictFail},
		{"exact content", FilesConfig{Equals: map[string]string{"src/main.go": "package main\n"}}, contract.VerdictPass},
		{"content differs", FilesConfig{Equals: map[string]string{"src/main.go": "package other\n"}}, contract.VerdictFail},
		{"nothing asserted", FilesConfig{}, contract.VerdictPass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Files{}.Grade(context.Background(), Input{
				Ref:       contract.GraderRef{Config: config(t, tt.cfg)},
				Workspace: ws,
			})
			if got.Verdict != tt.want {
				t.Errorf("verdict = %s (%s), want %s", got.Verdict, got.Explanation, tt.want)
			}
			if got.Verdict == contract.VerdictFail && got.Explanation == "" {
				t.Error("a failing grader must say what was wrong")
			}
		})
	}
}

// A task is portable, so it must be refused the same way on every host. The
// Windows forms are listed alongside the Unix ones deliberately: filepath.IsAbs
// only recognises the host's own, so a suite that ran on the other platform
// would otherwise silently resolve these inside the workspace.
func TestFilesGraderRefusesPathsOutsideTheWorkspace(t *testing.T) {
	ws := workspaceWith(t, map[string]string{"a.txt": "x\n"})
	for _, path := range []string{
		"../escape", "/etc/passwd", "sub/../../out",
		`\Windows\System32\drivers\etc\hosts`, `C:\Windows\System32`, "C:relative",
	} {
		got := Files{}.Grade(context.Background(), Input{
			Ref:       contract.GraderRef{Config: config(t, FilesConfig{Exists: []string{path}})},
			Workspace: ws,
		})
		// Reaching outside is the task's defect, so it is unknown rather than a
		// failing subject.
		if got.Verdict != contract.VerdictUnknown {
			t.Errorf("asserting on %q gave %s, want unknown", path, got.Verdict)
		}
	}
}

func TestFilesGraderReportsABadPatternAsItsOwnDefect(t *testing.T) {
	ws := workspaceWith(t, map[string]string{"a.txt": "x\n"})
	got := Files{}.Grade(context.Background(), Input{
		Ref:       contract.GraderRef{Config: config(t, FilesConfig{Matches: map[string]string{"a.txt": "([unclosed"}})},
		Workspace: ws,
	})
	if got.Verdict != contract.VerdictUnknown {
		t.Errorf("verdict = %s, want unknown: a broken pattern is not a failing subject", got.Verdict)
	}
	if got.Error == "" {
		t.Error("a grader that could not run must record why")
	}
}

func TestCommandGrader(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the scripts below are POSIX shell")
	}
	ws := workspaceWith(t, map[string]string{"report.md": "done\n"})
	taskDir := t.TempDir()
	graders := filepath.Join(taskDir, contract.GradersDir)
	if err := os.MkdirAll(graders, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(graders, name), []byte(body), 0o755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("pass.sh", "#!/bin/sh\ntest -f report.md\n")
	write("fail.sh", "#!/bin/sh\necho 'report.md is empty' >&2\nexit 1\n")

	tests := []struct {
		name string
		cfg  CommandConfig
		want contract.Verdict
	}{
		{"check passes", CommandConfig{Run: []string{"./pass.sh"}}, contract.VerdictPass},
		{"check fails", CommandConfig{Run: []string{"./fail.sh"}}, contract.VerdictFail},
		{"expected failure", CommandConfig{Run: []string{"./fail.sh"}, ExpectExit: 1}, contract.VerdictPass},
		{"missing command", CommandConfig{Run: []string{"./absent.sh"}}, contract.VerdictUnknown},
		{"no command configured", CommandConfig{}, contract.VerdictUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Command{}.Grade(context.Background(), Input{
				Ref:       contract.GraderRef{Config: config(t, tt.cfg)},
				Workspace: ws,
				TaskDir:   taskDir,
			})
			if got.Verdict != tt.want {
				t.Errorf("verdict = %s (explanation %q, error %q), want %s",
					got.Verdict, got.Explanation, got.Error, tt.want)
			}
		})
	}
}

func TestRootedIsPortable(t *testing.T) {
	// Anchored on every host, whichever host is running the test. filepath.IsAbs
	// alone recognises only the local forms, so a task refused on Linux would be
	// resolved inside the workspace on Windows and vice versa.
	for _, p := range []string{
		"/etc/passwd", `\Windows\System32`, `C:\data`, "C:data", "d:/data",
	} {
		if !rooted(p) {
			t.Errorf("rooted(%q) = false; a task carrying it would mean different things per platform", p)
		}
	}
	for _, p := range []string{
		"", "report.md", "sub/report.md", `sub\report.md`, "./check.sh", "..", "../escape",
	} {
		if rooted(p) {
			t.Errorf("rooted(%q) = true; it is relative to whatever resolves it", p)
		}
	}
}

func TestCommandGraderRunsInTheWorkspaceAndNotTheTask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the script below is POSIX shell")
	}
	ws := workspaceWith(t, map[string]string{"only-in-workspace.txt": "x\n"})
	taskDir := t.TempDir()
	graders := filepath.Join(taskDir, contract.GradersDir)
	if err := os.MkdirAll(graders, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// The script proves where it ran: the file exists only in the workspace.
	if err := os.WriteFile(filepath.Join(graders, "where.sh"),
		[]byte("#!/bin/sh\ntest -f only-in-workspace.txt\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := Command{}.Grade(context.Background(), Input{
		Ref:       contract.GraderRef{Config: config(t, CommandConfig{Run: []string{"./where.sh"}})},
		Workspace: ws,
		TaskDir:   taskDir,
	})
	if got.Verdict != contract.VerdictPass {
		t.Errorf("verdict = %s (%s); the grader did not run in the workspace", got.Verdict, got.Explanation)
	}
}

// writeTrace builds a trial directory holding a trace with the given records.
func writeTrace(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, contract.TraceFile), []byte(body), 0o644); err != nil {
		t.Fatalf("write trace: %v", err)
	}
	return dir
}

func TestTraceGrader(t *testing.T) {
	trialDir := writeTrace(t,
		`{"ts":"t","type":"run_start","run_id":"r","model":"m"}`,
		`{"ts":"t","type":"tool_start","tool":"Read","tool_call_id":"1"}`,
		`{"ts":"t","type":"tool_end","tool":"Read","tool_call_id":"1"}`,
		`{"ts":"t","type":"tool_start","tool":"Write","tool_call_id":"2"}`,
		`{"ts":"t","type":"tool_end","tool":"Write","tool_call_id":"2"}`,
		`{"ts":"t","type":"tool_denied","tool":"Bash","deny_reason":"policy denied Bash"}`,
		`{"ts":"t","type":"run_end","tool_calls":2}`,
	)
	bundle := contract.TrialBundle{TracePath: contract.TraceFile}

	zero := 0
	tests := []struct {
		name string
		cfg  TraceConfig
		want contract.Verdict
	}{
		{"required tool used", TraceConfig{RequiredTools: []string{"Write"}}, contract.VerdictPass},
		{"required tool unused", TraceConfig{RequiredTools: []string{"WebFetch"}}, contract.VerdictFail},
		{"forbidden tool untouched", TraceConfig{ForbiddenTools: []string{"WebFetch"}}, contract.VerdictPass},
		// Bash never ran — the policy blocked it — but the subject still
		// reached for it, which is what a trust case is asking about.
		{"forbidden tool attempted", TraceConfig{ForbiddenTools: []string{"Bash"}}, contract.VerdictFail},
		{"denial required and present", TraceConfig{RequireDenial: true}, contract.VerdictPass},
		{"denial forbidden but present", TraceConfig{ForbidDenial: true}, contract.VerdictFail},
		{"within call budget", TraceConfig{MaxToolCalls: 5}, contract.VerdictPass},
		{"over call budget", TraceConfig{MaxToolCalls: 1}, contract.VerdictFail},
		{"no compaction expected", TraceConfig{MaxCompactions: &zero}, contract.VerdictPass},
		{"nothing asserted", TraceConfig{}, contract.VerdictPass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Trace{}.Grade(context.Background(), Input{
				Ref:      contract.GraderRef{Config: config(t, tt.cfg)},
				TrialDir: trialDir,
				Bundle:   bundle,
			})
			if got.Verdict != tt.want {
				t.Errorf("verdict = %s (%s), want %s", got.Verdict, got.Explanation, tt.want)
			}
		})
	}
}

func TestTraceGraderWithoutATraceIsUnknownNotPass(t *testing.T) {
	got := Trace{}.Grade(context.Background(), Input{
		Ref:      contract.GraderRef{Config: config(t, TraceConfig{ForbiddenTools: []string{"Bash"}})},
		TrialDir: t.TempDir(),
		Bundle:   contract.TrialBundle{},
	})
	if got.Verdict != contract.VerdictUnknown {
		t.Errorf("verdict = %s, want unknown: tracing being off must not read as a clean run", got.Verdict)
	}
}

func TestTraceGraderRejectsACorruptTrace(t *testing.T) {
	trialDir := writeTrace(t, `{"type":"tool_start","tool":"Read"}`, `not json at all`)
	got := Trace{}.Grade(context.Background(), Input{
		Ref:      contract.GraderRef{Config: config(t, TraceConfig{RequiredTools: []string{"Read"}})},
		TrialDir: trialDir,
		Bundle:   contract.TrialBundle{TracePath: contract.TraceFile},
	})
	if got.Verdict != contract.VerdictUnknown {
		t.Errorf("verdict = %s, want unknown: an unreadable trace is not a failing subject", got.Verdict)
	}
}

func TestGradeAllCarriesGatingWeightAndRejectsUnknownGraders(t *testing.T) {
	ws := workspaceWith(t, map[string]string{"report.md": "x\n"})
	task := contract.Task{Graders: []contract.GraderRef{
		{Name: "files", Version: 1, Kind: contract.GraderDeterministic, Required: true, Critical: true,
			Config: config(t, FilesConfig{Exists: []string{"report.md"}})},
		{Name: "nonexistent", Version: 1, Kind: contract.GraderDeterministic, Required: true},
	}}

	results := Builtin().GradeAll(context.Background(), task, Input{Workspace: ws})
	if len(results) != 2 {
		t.Fatalf("got %d results, want one per grader", len(results))
	}
	if results[0].Verdict != contract.VerdictPass || !results[0].Required || !results[0].Critical {
		t.Errorf("first result lost its gating weight: %+v", results[0])
	}
	// A task naming a grader this build lacks has not been evaluated. Skipping
	// it would let the trial pass on whichever graders happened to exist.
	if results[1].Verdict != contract.VerdictUnknown || results[1].Error == "" {
		t.Errorf("an unregistered grader was not reported: %+v", results[1])
	}
	if got := contract.DecideStatus(results); got != contract.StatusGraderError {
		t.Errorf("DecideStatus = %s, want grader_error", got)
	}
}

// slowGrader takes long enough that millisecond timing can see it.
type slowGrader struct{}

func (slowGrader) Grade(context.Context, Input) contract.GraderResult {
	time.Sleep(5 * time.Millisecond)
	// Reporting a nonsense duration proves GradeAll is the one that times.
	return contract.GraderResult{Verdict: contract.VerdictPass, Duration: contract.Duration(999999)}
}

func TestGradeAllTimesTheGraderItself(t *testing.T) {
	registry := Registry{"slow": slowGrader{}}
	task := contract.Task{Graders: []contract.GraderRef{{Name: "slow", Required: true}}}

	results := registry.GradeAll(context.Background(), task, Input{})
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Duration <= 0 {
		t.Errorf("duration = %d, want the measured time", results[0].Duration)
	}
	if results[0].Duration.Duration() > 30*time.Second {
		t.Errorf("duration = %v; the grader's own claim was kept instead of the measurement",
			results[0].Duration.Duration())
	}
}

func TestGradeAllOnAPassingTaskDecidesPassed(t *testing.T) {
	ws := workspaceWith(t, map[string]string{"report.md": "# Report\n"})
	trialDir := writeTrace(t, `{"type":"tool_start","tool":"Write"}`)
	task := contract.Task{Graders: []contract.GraderRef{
		{Name: "files", Kind: contract.GraderDeterministic, Required: true,
			Config: config(t, FilesConfig{Matches: map[string]string{"report.md": "^# Report"}})},
		{Name: "trace", Kind: contract.GraderTrace, Required: true,
			Config: config(t, TraceConfig{RequiredTools: []string{"Write"}, ForbiddenTools: []string{"Bash"}})},
	}}

	results := Builtin().GradeAll(context.Background(), task, Input{
		Workspace: ws,
		TrialDir:  trialDir,
		Bundle:    contract.TrialBundle{TracePath: contract.TraceFile},
	})
	if got := contract.DecideStatus(results); got != contract.StatusPassed {
		t.Errorf("DecideStatus = %s, want passed; results: %+v", got, results)
	}
}
