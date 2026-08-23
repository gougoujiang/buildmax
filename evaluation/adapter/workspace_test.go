package adapter

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

// writeTask builds a task directory: visible state, hidden graders, hidden
// oracle.
func writeTask(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

func TestMaterializeCopiesOnlyTheVisibleState(t *testing.T) {
	task := writeTask(t, map[string]string{
		"task.json":             `{"id":"t"}`,
		"state/src/main.go":     "package main\n",
		"state/README.md":       "# hello\n",
		"graders/check.sh":      "#!/bin/sh\nexit 0\n",
		"oracle/solution.patch": "the answer\n",
		"notes-for-authors.md":  "do not ship\n",
	})
	workspace := filepath.Join(t.TempDir(), "ws")

	if err := Materialize(task, workspace); err != nil {
		t.Fatalf("Materialize: %v", err)
	}

	for _, want := range []string{"src/main.go", "README.md"} {
		if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(want))); err != nil {
			t.Errorf("visible state %s did not reach the workspace: %v", want, err)
		}
	}
	for _, forbidden := range []string{"task.json", "check.sh", "solution.patch", "notes-for-authors.md",
		contract.GradersDir, contract.OracleDir} {
		if _, err := os.Stat(filepath.Join(workspace, forbidden)); err == nil {
			t.Errorf("%s reached the workspace; the trial boundary leaked", forbidden)
		}
	}
}

func TestMaterializeWithoutStateIsAnEmptyWorkspace(t *testing.T) {
	task := writeTask(t, map[string]string{"task.json": `{"id":"t"}`})
	workspace := filepath.Join(t.TempDir(), "ws")

	if err := Materialize(task, workspace); err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("workspace holds %d entries, want an empty one", len(entries))
	}
}

func TestMaterializeRejectsSymlinkedState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	task := writeTask(t, map[string]string{"state/keep.txt": "x\n"})
	if err := os.Symlink("/etc/passwd", filepath.Join(task, contract.StateDir, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	err := Materialize(task, filepath.Join(t.TempDir(), "ws"))
	if err == nil {
		t.Fatal("Materialize accepted a symlink, which would point the agent outside the trial boundary")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error %q does not say what was wrong", err)
	}
}

func TestDigestDirIsStableAndSensitive(t *testing.T) {
	build := func(files map[string]string) string {
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
	digest := func(dir string) string {
		t.Helper()
		got, err := DigestDir(dir)
		if err != nil {
			t.Fatalf("DigestDir: %v", err)
		}
		return got
	}

	files := map[string]string{"a.txt": "one\n", "sub/b.txt": "two\n"}
	first := digest(build(files))

	// Same content in a separate tree must digest the same, or every re-run
	// would report a changed outcome.
	if second := digest(build(files)); second != first {
		t.Errorf("identical trees digested differently:\n%s\n%s", first, second)
	}

	changed := map[string]string{"a.txt": "one\n", "sub/b.txt": "TWO\n"}
	if got := digest(build(changed)); got == first {
		t.Error("a changed file left the digest unchanged")
	}

	added := map[string]string{"a.txt": "one\n", "sub/b.txt": "two\n", "c.txt": ""}
	if got := digest(build(added)); got == first {
		t.Error("an added file left the digest unchanged")
	}
}

func TestDigestDirNoticesAnEmptyDirectory(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	before, err := DigestDir(base)
	if err != nil {
		t.Fatalf("DigestDir: %v", err)
	}
	// "created the output directory" is an outcome a task can require, so an
	// empty directory has to move the digest.
	if err := os.Mkdir(filepath.Join(base, "out"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	after, err := DigestDir(base)
	if err != nil {
		t.Fatalf("DigestDir: %v", err)
	}
	if before == after {
		t.Error("creating an empty directory left the digest unchanged")
	}
}

func TestVerifyBoundaryCatchesLeakedGraderMaterial(t *testing.T) {
	const answer = "the grader checks for exactly this\n"
	task := writeTask(t, map[string]string{
		"graders/expected.txt": answer,
		"oracle/solution.txt":  "oracle body\n",
		"state/README.md":      "# task\n",
	})

	clean := filepath.Join(t.TempDir(), "clean")
	if err := Materialize(task, clean); err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	leaked, err := VerifyBoundary(task, clean)
	if err != nil {
		t.Fatalf("VerifyBoundary: %v", err)
	}
	if len(leaked) != 0 {
		t.Errorf("a correctly materialized workspace reported leaks: %v", leaked)
	}

	// A task author who commits the expected answer into the visible state
	// leaks it just as surely as a broken copier would.
	dirty := filepath.Join(t.TempDir(), "dirty")
	if err := Materialize(task, dirty); err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirty, "hint.txt"), []byte(answer), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	leaked, err = VerifyBoundary(task, dirty)
	if err != nil {
		t.Fatalf("VerifyBoundary: %v", err)
	}
	if len(leaked) != 1 || leaked[0] != "hint.txt" {
		t.Errorf("leaks = %v, want [hint.txt]", leaked)
	}
}

func TestVerifyBoundaryIgnoresEmptyFiles(t *testing.T) {
	task := writeTask(t, map[string]string{"graders/placeholder": "", "state/out": ""})
	workspace := filepath.Join(t.TempDir(), "ws")
	if err := Materialize(task, workspace); err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	leaked, err := VerifyBoundary(task, workspace)
	if err != nil {
		t.Fatalf("VerifyBoundary: %v", err)
	}
	if len(leaked) != 0 {
		t.Errorf("empty files matched as a leak: %v", leaked)
	}
}

func TestWriteHomeCarriesOnlyTheSubject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "home")
	subject := contract.SubjectManifest{
		Name: "candidate",
		Model: contract.ModelIdentity{
			Transport:     "openai_compatible",
			Target:        "test/model",
			ContextWindow: 32000,
		},
	}
	if err := WriteHome(dir, subject, Credential{APIURL: "http://127.0.0.1:1/v1", APIKey: "k"}); err != nil {
		t.Fatalf("WriteHome: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "settings.yaml"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	body := string(data)
	for _, want := range []string{"model: test/model", "provider: openai_compatible", "context_window: 32000"} {
		if !strings.Contains(body, want) {
			t.Errorf("settings missing %q:\n%s", want, body)
		}
	}
	// A trial home the subject did not ask for is a subject the manifest does
	// not describe.
	for _, forbidden := range []string{"hooks:", "plugins:", "sandbox:", "tools:"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("settings carry %q, which no subject field asked for:\n%s", forbidden, body)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "plugins")); err == nil {
		t.Error("trial home has a plugins directory, so an installed plugin could join the run")
	}
}

func TestWriteHomeRefusesTheManagedGateway(t *testing.T) {
	subject := contract.SubjectManifest{
		Name:  "managed",
		Model: contract.ModelIdentity{Transport: "buildmax", Target: "default"},
	}
	err := WriteHome(filepath.Join(t.TempDir(), "home"), subject, Credential{})
	if err == nil {
		t.Fatal("WriteHome accepted a managed subject, which it would have measured as a direct one")
	}
}

func TestStatusForExit(t *testing.T) {
	tests := []struct {
		code    int
		want    contract.TrialStatus
		decided bool
	}{
		{0, "", false},
		{3, "", false}, // policy denied: a trust task may require it
		{4, contract.StatusAgentError, true},
		{6, contract.StatusCanceled, true},
		{2, contract.StatusInfrastructureError, true},
		{1, contract.StatusAgentError, true},
	}
	for _, tt := range tests {
		got, decided := statusForExit(tt.code)
		if got != tt.want || decided != tt.decided {
			t.Errorf("statusForExit(%d) = (%q, %v), want (%q, %v)", tt.code, got, decided, tt.want, tt.decided)
		}
	}
}

func TestParseEnvelopeReportsUnparsableOutput(t *testing.T) {
	if _, err := parseEnvelope(nil); err == nil {
		t.Error("parseEnvelope accepted empty output")
	}
	if _, err := parseEnvelope([]byte("panic: something\n")); err == nil {
		t.Error("parseEnvelope accepted a crash message as a result")
	}
	env, err := parseEnvelope([]byte(`{"session_id":"s","trace_id":"r","exit_code":0}`))
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	if env.SessionID != "s" || env.TraceID != "r" {
		t.Errorf("parsed %+v, want session s and trace r", env)
	}
}
