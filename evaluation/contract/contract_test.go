package contract

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectDigestIgnoresID(t *testing.T) {
	base := SubjectManifest{
		ContractVersion: Version,
		Name:            "candidate",
		Build:           BuildIdentity{Version: "0.1.0", Commit: "abc1234", ArtifactDigest: "sha256:aaa"},
		Model:           ModelIdentity{Transport: "anthropic", Target: "claude-opus-5"},
	}
	withID, err := base.WithID()
	if err != nil {
		t.Fatalf("WithID: %v", err)
	}
	if withID.ID == "" {
		t.Fatal("WithID left the id empty")
	}

	// Re-digesting a manifest that already carries its id must reproduce it;
	// otherwise the id could never be verified against the manifest it names.
	again, err := withID.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if again != withID.ID {
		t.Errorf("digest of a stamped manifest = %s, want %s", again, withID.ID)
	}
}

func TestSubjectDigestSeparatesConfigurations(t *testing.T) {
	base := SubjectManifest{Name: "s", Model: ModelIdentity{Target: "m"}}
	sandboxed := true

	cases := map[string]func(*SubjectManifest){
		"model target":    func(m *SubjectManifest) { m.Model.Target = "other" },
		"artifact digest": func(m *SubjectManifest) { m.Build.ArtifactDigest = "sha256:bbb" },
		"dirty tree":      func(m *SubjectManifest) { m.Build.Dirty = "sha256:ccc" },
		"transport":       func(m *SubjectManifest) { m.Model.Transport = "gateway" },
		"adapter version": func(m *SubjectManifest) { m.Execution.AdapterVersion = 2 },
		"sandbox":         func(m *SubjectManifest) { m.Policy.Sandboxed = &sandboxed },
		"prompt digest":   func(m *SubjectManifest) { m.Instructions.SystemPromptDigest = "sha256:ddd" },
		"plugins":         func(m *SubjectManifest) { m.Extensions.Plugins = []string{"p"} },
		"host arch":       func(m *SubjectManifest) { m.Host.Arch = "arm64" },
	}

	original, err := base.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			got, err := changed.Digest()
			if err != nil {
				t.Fatalf("Digest: %v", err)
			}
			if got == original {
				t.Errorf("changing the %s left the subject id unchanged, so two configurations would compare as paired", name)
			}
		})
	}
}

func TestStatusClassification(t *testing.T) {
	tests := []struct {
		status  TrialStatus
		scored  bool
		harness bool
	}{
		{StatusPassed, true, false},
		{StatusFailed, true, false},
		{StatusTimedOut, true, false},
		{StatusAgentError, false, false},
		{StatusInfrastructureError, false, true},
		{StatusGraderError, false, true},
		{StatusInvalidTask, false, true},
		{StatusCanceled, false, false},
	}
	for _, tt := range tests {
		if got := tt.status.Scored(); got != tt.scored {
			t.Errorf("%s.Scored() = %v, want %v", tt.status, got, tt.scored)
		}
		if got := tt.status.HarnessFault(); got != tt.harness {
			t.Errorf("%s.HarnessFault() = %v, want %v", tt.status, got, tt.harness)
		}
	}
}

func TestDecideStatus(t *testing.T) {
	required := func(v Verdict) GraderResult {
		return GraderResult{Name: "state", Required: true, Kind: GraderDeterministic, Verdict: v}
	}
	optional := func(v Verdict) GraderResult {
		return GraderResult{Name: "style", Kind: GraderModel, Verdict: v}
	}

	tests := []struct {
		name    string
		results []GraderResult
		want    TrialStatus
	}{
		{"all required pass", []GraderResult{required(VerdictPass), required(VerdictPass)}, StatusPassed},
		{"optional failure does not gate", []GraderResult{required(VerdictPass), optional(VerdictFail)}, StatusPassed},
		{"required failure", []GraderResult{required(VerdictPass), required(VerdictFail)}, StatusFailed},
		{"required unknown is unscored", []GraderResult{required(VerdictPass), required(VerdictUnknown)}, StatusGraderError},
		{"optional unknown does not gate", []GraderResult{required(VerdictPass), optional(VerdictUnknown)}, StatusPassed},
		{"definite failure beats inconclusive", []GraderResult{required(VerdictUnknown), required(VerdictFail)}, StatusFailed},
		{"grader error is unscored", []GraderResult{{Name: "x", Required: true, Error: "boom"}}, StatusGraderError},
		{"no graders", nil, StatusPassed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecideStatus(tt.results); got != tt.want {
				t.Errorf("DecideStatus = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCriticalFailuresNamed(t *testing.T) {
	results := []GraderResult{
		{Name: "state", Required: true, Verdict: VerdictFail},
		{Name: "no-network", Required: true, Critical: true, Verdict: VerdictFail},
		{Name: "no-secrets", Required: true, Critical: true, Verdict: VerdictPass},
	}
	got := CriticalFailures(results)
	if len(got) != 1 || got[0] != "no-network" {
		t.Errorf("CriticalFailures = %v, want [no-network]", got)
	}
}

func TestMaterializedKeepsGradersOutOfTheWorkspace(t *testing.T) {
	if !Materialized(StateDir) {
		t.Errorf("%s must reach the trial workspace", StateDir)
	}
	for _, name := range []string{GradersDir, OracleDir, TaskFile, "notes", "solution.patch"} {
		if Materialized(name) {
			t.Errorf("%s must stay behind the trial boundary", name)
		}
	}
}

func TestBundleRoundTrip(t *testing.T) {
	root := t.TempDir()
	cost := int64(1_500_000)
	want := TrialBundle{
		TrialID:            "tr_1",
		ExperimentID:       "ex_1",
		TaskID:             "local-write-report",
		TaskVersion:        3,
		Suite:              "local-workbench",
		SubjectID:          "sha256:subject",
		Index:              2,
		Domain:             DomainCapability,
		Surface:            SurfaceCLI,
		Status:             StatusFailed,
		FailureClass:       "claimed-without-writing",
		StartedAt:          time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC),
		Duration:           FromDuration(90 * time.Second),
		InitialStateDigest: "sha256:before",
		FinalStateDigest:   "sha256:after",
		Retention:          RetentionBounded,
		Reply:              "I wrote the report.",
		TracePath:          TraceFile,
		Artifacts:          []ArtifactRef{{Name: "report.md", Digest: "sha256:art", Bytes: 42}},
		Graders: []GraderResult{
			{Name: "file-exists", Version: 1, Kind: GraderDeterministic, Required: true, Verdict: VerdictFail,
				Explanation: "report.md is absent"},
		},
		Usage:     Usage{LLMCalls: 4, ToolCalls: 7, PromptTokens: 1200, CompletionTokens: 300, Cost: &cost, Currency: "USD"},
		Reproduce: Reproduction{Command: []string{"buildmax", "-p", "..."}, Dataset: DatasetRef{Name: "public", Version: "1"}},
	}

	dir, err := WriteBundle(root, want)
	if err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}
	got, err := ReadBundle(dir)
	if err != nil {
		t.Fatalf("ReadBundle: %v", err)
	}

	if got.ContractVersion != Version {
		t.Errorf("contract version = %d, want %d", got.ContractVersion, Version)
	}
	want.ContractVersion = Version
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("started_at = %v, want %v", got.StartedAt, want.StartedAt)
	}
	got.StartedAt, want.StartedAt = time.Time{}, time.Time{}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("round trip changed the bundle:\n got %s\nwant %s", gotJSON, wantJSON)
	}
	if got.Duration.Duration() != 90*time.Second {
		t.Errorf("duration = %v, want 90s", got.Duration.Duration())
	}
}

func TestDurationSerialisesAsMilliseconds(t *testing.T) {
	data, err := json.Marshal(struct {
		D Duration `json:"d"`
	}{FromDuration(1500 * time.Millisecond)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != `{"d":1500}` {
		t.Errorf("encoded %s, want {\"d\":1500}", data)
	}
}

func TestReadRejectsUnknownContractVersion(t *testing.T) {
	root := t.TempDir()
	dir, err := WriteBundle(root, TrialBundle{TaskID: "t", Index: 0})
	if err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}

	path := filepath.Join(dir, BundleFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	bumped := strings.Replace(string(data),
		`"contract_version": 1`, `"contract_version": 99`, 1)
	if bumped == string(data) {
		t.Fatal("test could not bump the stored contract version")
	}
	if err := os.WriteFile(path, []byte(bumped), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := ReadBundle(dir); !errors.Is(err, ErrVersion) {
		t.Errorf("ReadBundle error = %v, want ErrVersion", err)
	}
}

func TestReadBundlesOrdersByTaskThenIndex(t *testing.T) {
	root := t.TempDir()
	// Written out of order, and with indices whose lexical and numeric orders
	// disagree, which is what a directory listing would get wrong.
	for _, b := range []TrialBundle{
		{TaskID: "beta", Index: 10},
		{TaskID: "alpha", Index: 9},
		{TaskID: "beta", Index: 9},
		{TaskID: "alpha", Index: 10},
	} {
		if _, err := WriteBundle(root, b); err != nil {
			t.Fatalf("WriteBundle: %v", err)
		}
	}

	got, err := ReadBundles(root)
	if err != nil {
		t.Fatalf("ReadBundles: %v", err)
	}
	var order []string
	for _, b := range got {
		order = append(order, b.TaskID+"/"+string(rune('0'+b.Index/10))+string(rune('0'+b.Index%10)))
	}
	want := []string{"alpha/09", "alpha/10", "beta/09", "beta/10"}
	if len(order) != len(want) {
		t.Fatalf("read %d bundles, want %d", len(order), len(want))
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order = %v, want %v", order, want)
			break
		}
	}
}

func TestReadBundlesOnEmptyTree(t *testing.T) {
	got, err := ReadBundles(t.TempDir())
	if err != nil {
		t.Fatalf("ReadBundles on an empty tree: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("read %d bundles from an empty tree", len(got))
	}
}

func TestTrialDirRejectsEscapingTaskIDs(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../escape", "a/b", `a\b`, "a b"} {
		if _, err := TrialDir(t.TempDir(), id, 0); err == nil {
			t.Errorf("TrialDir accepted task id %q, which would place evidence outside the tree", id)
		}
	}
	if _, err := TrialDir(t.TempDir(), "local-write.report_2", 0); err != nil {
		t.Errorf("TrialDir rejected a valid task id: %v", err)
	}
	if _, err := TrialDir(t.TempDir(), "ok", -1); err == nil {
		t.Error("TrialDir accepted a negative trial index")
	}
}

func TestExperimentRoundTrip(t *testing.T) {
	root := t.TempDir()
	subject, err := SubjectManifest{Name: "candidate", Model: ModelIdentity{Target: "m"}}.WithID()
	if err != nil {
		t.Fatalf("WithID: %v", err)
	}
	want := Experiment{
		ID:        "ex_1",
		Name:      "nightly",
		CreatedAt: time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC),
		Dataset:   DatasetRef{Name: "public", Version: "1", Digest: "sha256:ds"},
		Subjects:  []SubjectManifest{subject},
		Baseline:  subject.ID,
		Trials:    5,
		Tasks:     []string{"local-write-report"},
	}
	if err := WriteExperiment(root, want); err != nil {
		t.Fatalf("WriteExperiment: %v", err)
	}
	got, err := ReadExperiment(root)
	if err != nil {
		t.Fatalf("ReadExperiment: %v", err)
	}
	if got.ContractVersion != Version || got.ID != want.ID || got.Trials != want.Trials {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
	if len(got.Subjects) != 1 || got.Subjects[0].ID != subject.ID {
		t.Errorf("subjects did not survive the round trip: %+v", got.Subjects)
	}
}
