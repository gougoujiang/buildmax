package harbor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Harbor's own on-disk layout. A job directory holds one directory per trial,
// and each of those holds the files below.
const (
	// TrialResultFile is Harbor's record of one attempt.
	//
	// Singular, despite Harbor's own TrialPaths docstring calling it
	// results.json in two places: its `result_path` property returns
	// result.json, and that is what a real job directory holds. The code is the
	// fact.
	TrialResultFile = "result.json"
	// TrialConfigFile is the configuration that attempt ran under. Harbor
	// writes it with defaults excluded, so an absent field means the default.
	TrialConfigFile = "config.json"
	// TrialAgentDir is where the agent's own logs land, mounted from the
	// container's /logs/agent. It is where this repository's adapter leaves
	// BuildMax's result envelope and the run's traces.
	TrialAgentDir = "agent"
	// AgentResultFile is the print-mode envelope the BuildMax adapter writes.
	AgentResultFile = "buildmax-result.json"
	// AgentSessionsDir holds the durable traces the adapter copied out.
	AgentSessionsDir = "sessions"
)

// These types read files this repository does not own, so unlike its own JSON
// they accept unknown fields. Harbor's result model carries far more than a
// BuildMax bundle needs and grows between releases; refusing a field nobody
// here reads would make every upstream release break the import of evidence
// that is otherwise perfectly legible.

// TrialResult is the part of Harbor's results.json this package reads.
type TrialResult struct {
	TaskName  string `json:"task_name"`
	TrialName string `json:"trial_name"`
	TrialURI  string `json:"trial_uri"`
	// Source is the dataset the task came from, absent for an ad-hoc run.
	Source *string `json:"source"`
	// TaskChecksum pins the task content the attempt started from. It is the
	// initial-state identity for an external benchmark: BuildMax never
	// materialized the workspace, so the task's own digest is what says where
	// the trial began.
	TaskChecksum string `json:"task_checksum"`

	AgentInfo      AgentInfo       `json:"agent_info"`
	AgentResult    *AgentContext   `json:"agent_result"`
	VerifierResult *VerifierResult `json:"verifier_result"`
	ExceptionInfo  *ExceptionInfo  `json:"exception_info"`

	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// AgentInfo identifies the agent that ran, as Harbor recorded it.
type AgentInfo struct {
	Name      string     `json:"name"`
	Version   string     `json:"version"`
	ModelInfo *ModelInfo `json:"model_info"`
}

// ModelInfo is Harbor's model identity. Provider is absent when the run named a
// model without a `provider/` prefix, which Harbor records rather than filling
// in — the same choice the BuildMax subject manifest makes about a revision.
type ModelInfo struct {
	Name     string  `json:"name"`
	Provider *string `json:"provider"`
}

// AgentContext is what the agent reported about its own run. For the BuildMax
// adapter, Metadata carries the subject facts the adapter resolved and the
// print-mode envelope's exit classification.
type AgentContext struct {
	InputTokens  *int           `json:"n_input_tokens"`
	CacheTokens  *int           `json:"n_cache_tokens"`
	OutputTokens *int           `json:"n_output_tokens"`
	CostUSD      *float64       `json:"cost_usd"`
	Metadata     map[string]any `json:"metadata"`
}

// VerifierResult is the benchmark's own verdict. Rewards is an open map because
// a task decides its own keys: a Terminal-Bench task writes reward.txt, which
// Harbor reads as a single "reward" of 0 or 1.
type VerifierResult struct {
	Rewards map[string]float64 `json:"rewards"`
}

// ExceptionInfo is how a trial failed outside the verifier's judgement.
type ExceptionInfo struct {
	Type    string `json:"exception_type"`
	Message string `json:"exception_message"`
}

// TrialConfig is the part of Harbor's config.json this package reads: what the
// attempt was configured to run, as opposed to what it reported afterwards.
type TrialConfig struct {
	Agent struct {
		Name       string         `json:"name"`
		ImportPath string         `json:"import_path"`
		ModelName  string         `json:"model_name"`
		Kwargs     map[string]any `json:"kwargs"`
	} `json:"agent"`
}

// Trial is one attempt as Harbor left it on disk.
type Trial struct {
	// Dir is the trial directory, kept so evidence beside the manifests can be
	// found without recomputing where it was.
	Dir    string
	Result TrialResult
	Config TrialConfig
}

// AgentDir is where the agent's logs landed.
func (t Trial) AgentDir() string { return filepath.Join(t.Dir, TrialAgentDir) }

// LoadJob reads every trial in a Harbor job directory, ordered by task and then
// by start time.
//
// A directory without a result file is skipped: Harbor creates a trial
// directory before the trial runs, so an interrupted job leaves empties behind
// and refusing them would make a partial job unreadable. A result file that
// does not parse is an error, because that is evidence this build cannot
// interpret rather than evidence that was never written.
func LoadJob(dir string) ([]Trial, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read job dir: %w", err)
	}

	var trials []Trial
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		trialDir := filepath.Join(dir, e.Name())
		resultPath := filepath.Join(trialDir, TrialResultFile)
		if _, err := os.Stat(resultPath); err != nil {
			continue
		}
		var result TrialResult
		if err := readForeignJSON(resultPath, &result); err != nil {
			return nil, err
		}
		var config TrialConfig
		// Absent is not an error: a trial that failed before its configuration
		// was written still recorded a result worth importing.
		configPath := filepath.Join(trialDir, TrialConfigFile)
		if _, err := os.Stat(configPath); err == nil {
			if err := readForeignJSON(configPath, &config); err != nil {
				return nil, err
			}
		}
		trials = append(trials, Trial{Dir: trialDir, Result: result, Config: config})
	}

	if len(trials) == 0 {
		return nil, fmt.Errorf("no trial results under %s; is it a Harbor job directory?", dir)
	}
	sortTrials(trials)
	return trials, nil
}

// sortTrials orders attempts deterministically: by task, then by when they
// started, then by Harbor's own trial name.
//
// The ordering matters because Harbor does not number attempts. A trial name is
// the task plus a random suffix, so the attempt index a bundle carries is
// assigned here. Two imports of the same job must assign the same indices or a
// paired comparison would pair different attempts each time it ran.
func sortTrials(trials []Trial) {
	sort.SliceStable(trials, func(i, j int) bool {
		a, b := trials[i].Result, trials[j].Result
		if a.TaskName != b.TaskName {
			return a.TaskName < b.TaskName
		}
		at, bt := startTime(a), startTime(b)
		if !at.Equal(bt) {
			return at.Before(bt)
		}
		return a.TrialName < b.TrialName
	})
}

func startTime(r TrialResult) time.Time {
	if r.StartedAt == nil {
		return time.Time{}
	}
	return *r.StartedAt
}

// readForeignJSON decodes a file this repository does not own, accepting fields
// it does not know. See the note above the types.
func readForeignJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
