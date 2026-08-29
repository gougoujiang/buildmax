package harbor

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gougoujiang/buildmax/evaluation/contract"
	"github.com/gougoujiang/buildmax/evaluation/trace"
)

// VerifierGrader is the single grader a Harbor trial carries. There is exactly
// one because section 14.2 leaves the benchmark's own verifier authoritative:
// BuildMax records what it decided and does not re-grade the workspace, which
// it never saw.
const VerifierGrader = "harbor-verifier"

// PassingReward is the value a Terminal-Bench task writes for success. Its
// tasks write 0 or 1 to reward.txt, so anything short of full credit is a
// failure rather than a partial pass — the recorded score keeps the number for
// a task that ever reports one in between.
const PassingReward = 1.0

// rewardKey is the key Harbor uses when a task reports a single reward through
// reward.txt, which every Terminal-Bench task does.
const rewardKey = "reward"

const nanoUnitsPerUnit = 1_000_000_000

// The subject facts the Python adapter records on each trial, because it is the
// code that resolved them. Deriving the BuildMax protocol here from Harbor's
// provider slug would re-implement the adapter's mapping in a second language,
// and the two would drift the first time either side learned a provider.
//
// A test holds the adapter to these names: a key spelled differently on one
// side is not an error, it is a field that silently reads as absent.
const (
	metaProvider      = "buildmax_provider"
	metaReasoning     = "reasoning"
	metaContextWindow = "context_window"
	metaMaxOutput     = "max_output"
	metaPermissions   = "permissions"
	metaSandboxed     = "sandboxed"
	metaArtifact      = "artifact_digest"
)

// subjectMetadataKeys is the set above, for the parity test.
var subjectMetadataKeys = []string{
	metaProvider, metaReasoning, metaContextWindow, metaMaxOutput,
	metaPermissions, metaSandboxed, metaArtifact,
}

// SubjectInput is what a Harbor job cannot say about the subject it measured.
//
// Only the host, and only because trials execute in containers Harbor placed
// somewhere: the machine that started them is what a latency comparison has to
// hold constant, and nothing in a job directory records it.
//
// The artifact is deliberately not here. Harbor records the agent kwarg naming
// a binary path, which is not the digest of the file that ran, and a caller
// asserting the digest afterwards can assert the wrong one with nothing able to
// check it. The adapter digests the binary it uploads and records that instead.
type SubjectInput struct {
	Name string
	Host contract.HostProfile
}

// Conversion is one Harbor job expressed in the BuildMax contract.
type Conversion struct {
	Subject contract.SubjectManifest
	Bundles []contract.TrialBundle
}

// Convert turns a loaded Harbor job into one bundle per attempt.
//
// It reads two sources per trial and prefers the closer one. Harbor's
// results.json is a re-encoding of what the agent reported — its cost is a
// float in dollars, which cannot round-trip the runtime's integer nano-units —
// while the print-mode envelope the adapter left in the trial's agent directory
// is BuildMax's own first-hand report. The envelope wins where both speak; the
// verifier's verdict is only ever Harbor's.
func Convert(trials []Trial, pins Pins, opt Options) (Conversion, error) {
	if len(trials) == 0 {
		return Conversion{}, fmt.Errorf("no trials to convert")
	}
	if err := checkDataset(trials, pins); err != nil {
		return Conversion{}, err
	}

	subject, err := buildSubject(trials, pins, opt.Subject)
	if err != nil {
		return Conversion{}, err
	}

	// LoadJob has already ordered attempts; the index is assigned from that
	// order because Harbor does not number them. See sortTrials.
	// Keyed on the bundle's own task id rather than Harbor's qualified name, so
	// the numbering and the directory it lands in cannot disagree.
	indices := map[string]int{}
	bundles := make([]contract.TrialBundle, 0, len(trials))
	for _, t := range trials {
		id := taskID(t.Result.TaskName)
		index := indices[id]
		indices[id] = index + 1
		bundle, err := convertTrial(t, pins, subject, opt, index)
		if err != nil {
			return Conversion{}, err
		}
		bundles = append(bundles, bundle)
	}
	return Conversion{Subject: subject, Bundles: bundles}, nil
}

// checkDataset refuses a job that measured something other than the pin.
//
// Importing it would file evidence from one dataset under another's name, and
// a 2.0 result read as a 2.1 result is exactly the comparison section 14.2
// forbids: the two differ by corrected tasks, not only by the agent.
func checkDataset(trials []Trial, pins Pins) error {
	for _, t := range trials {
		if t.Result.Source == nil || *t.Result.Source == "" {
			continue
		}
		// Harbor records the source as name or name@ref; the pin names the
		// dataset, and the ref is checked by the run rather than re-derived.
		name, _, _ := strings.Cut(*t.Result.Source, "@")
		if name != pins.Dataset.Name {
			return fmt.Errorf("trial %s ran dataset %q, but this repository is pinned to %q",
				t.Result.TrialName, name, pins.Dataset.Name)
		}
	}
	return nil
}

func buildSubject(trials []Trial, pins Pins, in SubjectInput) (contract.SubjectManifest, error) {
	// The first trial that reported anything, not simply the first: a job can
	// begin with an attempt whose container never started, and that one knows
	// nothing about the subject while every other attempt in the same job does.
	first := trials[0]
	for _, t := range trials {
		if metadataOf(t) != nil {
			first = t
			break
		}
	}
	meta := metadataOf(first)

	model := contract.ModelIdentity{
		Transport:     stringField(meta, metaProvider),
		Target:        modelTarget(first),
		Reasoning:     stringField(meta, metaReasoning),
		ContextWindow: intField(meta, metaContextWindow),
		MaxOutput:     intField(meta, metaMaxOutput),
	}
	if model.Transport == "" {
		// The adapter records the protocol it resolved. Deriving it here from
		// Harbor's provider slug would re-implement that mapping in a second
		// language, and the two would drift the first time either side learned
		// a provider.
		return contract.SubjectManifest{}, fmt.Errorf(
			"trial %s records no buildmax_provider; it was produced by an adapter older than this importer",
			first.Result.TrialName)
	}
	if model.Target == "" {
		return contract.SubjectManifest{}, fmt.Errorf("trial %s names no model", first.Result.TrialName)
	}

	build, err := buildIdentity(first, meta)
	if err != nil {
		return contract.SubjectManifest{}, err
	}

	policy := contract.PolicyResolution{Permissions: stringField(meta, metaPermissions)}
	if sandboxed, ok := meta[metaSandboxed].(bool); ok {
		// Left nil when the adapter did not state it. An unreported boundary is
		// indistinguishable from an unresolved one, and writing false for it
		// would claim knowledge the evidence does not carry.
		policy.Sandboxed = &sandboxed
	}

	return contract.SubjectManifest{
		ContractVersion: contract.Version,
		Name:            in.Name,
		Build:           build,
		Execution: contract.ExecutionIdentity{
			Surface:        contract.SurfaceHarbor,
			AdapterVersion: pins.Adapter.Version,
		},
		Model:  model,
		Policy: policy,
		Host:   in.Host,
		Dataset: contract.DatasetRef{
			Name: pins.Dataset.Name,
			// The harness version is the dataset's version here: Harbor
			// resolves the ref, so a result is only reproducible against the
			// release that resolved it.
			Version: "harbor " + pins.Harbor.Version,
			Digest:  pins.Dataset.Ref,
		},
	}.WithID()
}

// taskID is Harbor's task name as one path element.
//
// Harbor names a packaged task `<org>/<name>`, and a bundle's task id becomes a
// directory: contract.TrialDir rejects a separator outright, so importing the
// qualified name would fail on every trial. The org is already pinned by the
// dataset the subject records, so dropping it here loses nothing — and it is
// what Harbor's own trial naming does.
func taskID(taskName string) string {
	if at := strings.LastIndex(taskName, "/"); at >= 0 {
		return taskName[at+1:]
	}
	return taskName
}

// stateDigest normalizes Harbor's task checksum.
//
// It arrives as bare hex on the trial result, while the same tree's ref in the
// trial configuration is written `sha256:…`. A bundle's digests are compared
// and read beside each other, so they carry one shape; an unlabelled hex string
// does not even say what it is a digest of.
func stateDigest(checksum string) string {
	if checksum == "" || strings.Contains(checksum, ":") {
		return checksum
	}
	return "sha256:" + checksum
}

// buildIdentity is the artifact under evaluation, entirely from evidence.
//
// The digest comes from the adapter, which hashed the file it uploaded. The
// version and commit come from Harbor's agent_info, which is what the binary
// itself reported inside the container when Harbor asked it for a version — so
// all three describe the thing that ran rather than the tree it was built from.
func buildIdentity(t Trial, meta map[string]any) (contract.BuildIdentity, error) {
	digest := stringField(meta, metaArtifact)
	if digest == "" {
		return contract.BuildIdentity{}, fmt.Errorf(
			"trial %s records no %s; without it the result names a revision rather than the artifact that ran",
			t.Result.TrialName, metaArtifact)
	}
	version, commit := splitVersion(t.Result.AgentInfo.Version)
	return contract.BuildIdentity{
		Version:        version,
		Commit:         commit,
		ArtifactDigest: digest,
	}, nil
}

// splitVersion parses what `buildmax --version` reports, which the adapter
// hands to Harbor: "1.2.3 (abc1234)". An untagged build reports "dev", and the
// commit is what tells two dev builds apart, so it is kept separately rather
// than left inside one string.
func splitVersion(reported string) (version, commit string) {
	reported = strings.TrimSpace(reported)
	version, rest, found := strings.Cut(reported, " (")
	if !found {
		return reported, ""
	}
	return strings.TrimSpace(version), strings.TrimSuffix(rest, ")")
}

// modelTarget rebuilds the provider-qualified model name the run was given.
// Harbor splits it, and the two halves separately are not the coordinate a
// comparison holds constant.
func modelTarget(t Trial) string {
	if t.Config.Agent.ModelName != "" {
		return t.Config.Agent.ModelName
	}
	info := t.Result.AgentInfo.ModelInfo
	if info == nil {
		return ""
	}
	if info.Provider != nil && *info.Provider != "" {
		return *info.Provider + "/" + info.Name
	}
	return info.Name
}

func convertTrial(t Trial, pins Pins, subject contract.SubjectManifest, opt Options, index int) (contract.TrialBundle, error) {
	res := t.Result
	bundle := contract.TrialBundle{
		ContractVersion: contract.Version,
		TrialID:         res.TrialName,
		ExperimentID:    opt.ExperimentID,
		TaskID:          taskID(res.TaskName),
		// Harbor versions a task by content, not by number, and the checksum is
		// already carried as the initial state below. One is the version this
		// contract has room for.
		TaskVersion: 1,
		Suite:       pins.Dataset.Name,
		SubjectID:   subject.ID,
		Index:       index,
		// Terminal-Bench is an external capability benchmark and nothing else;
		// section 14.2 is explicit that it cannot establish worker governance,
		// Portal delivery, or the other product outcomes.
		Domain:  contract.DomainCapability,
		Surface: contract.SurfaceHarbor,
		// BuildMax never materialized the workspace, so the task's own content
		// digest is what says where this attempt began.
		InitialStateDigest: stateDigest(res.TaskChecksum),
		Retention:          opt.retention(),
		Reproduce: contract.Reproduction{
			Command:     reproduceCommand(t, pins),
			Dataset:     subject.Dataset,
			Note:        "reconstructed from the trial's recorded configuration, not captured verbatim; run it from evaluation/harbor",
			Environment: map[string]string{"HARBOR_VERSION": pins.Harbor.Version},
		},
	}
	if res.StartedAt != nil {
		bundle.StartedAt = *res.StartedAt
		if res.FinishedAt != nil {
			bundle.Duration = contract.FromDuration(res.FinishedAt.Sub(*res.StartedAt))
		}
	}

	envelope, envelopeGap := readEnvelope(t)
	applyUsage(&bundle, res, envelope, opt.retention())
	applyTrace(&bundle, t, envelope)

	grader, verdictKnown := verifierResult(res, pins)
	bundle.Graders = []contract.GraderResult{grader}
	status, failureClass, errText := decideStatus(res, envelope, grader, verdictKnown)
	bundle.Status = status
	bundle.FailureClass = failureClass
	// The trial's own reason first, then anything the evidence collection above
	// could not do. Overwriting here would drop whichever ran second.
	bundle.Error = appendReason(errText, appendReason(envelopeGap, bundle.Error))
	return bundle, nil
}

// decideStatus settles what one attempt says about the subject.
//
// The verifier decides whenever it reached a verdict, even for a trial that
// also recorded an exception: Harbor runs the tests against whatever the agent
// left behind, and a graded workspace is a real outcome however the agent's
// process ended. The exception is kept as the failure class so the reason is
// not lost.
//
// Only when there is no verdict does the exception classify the trial, and then
// the taxonomy matters more than the message: section 7.4 exists so a container
// that would not start is not reported as an agent that could not do the task.
func decideStatus(res TrialResult, envelope *agentEnvelope, grader contract.GraderResult, verdictKnown bool) (contract.TrialStatus, string, string) {
	failureClass := ""
	errText := ""
	if res.ExceptionInfo != nil {
		failureClass = res.ExceptionInfo.Type
		errText = res.ExceptionInfo.Message
	}
	// The adapter swallows the iteration cap so the verifier still judges the
	// work, so the cap arrives as a fact in the envelope rather than as an
	// exception. Naming it is what keeps a spent budget out of the capability
	// reading, per section 7.4.
	if envelope != nil && envelope.stoppedAtIterationCap() && failureClass == "" {
		failureClass = "iteration-cap"
	}

	if verdictKnown {
		status := contract.DecideStatus([]contract.GraderResult{grader})
		// One exception outranks a verdict, and only when the verdict is a
		// failure. StatusFailed means execution completed and a grader said no;
		// a run cut off at its budget did not complete, and reporting it as a
		// plain failure loses the reason. Both statuses are scored, so this
		// moves no trial in or out of the rate — it decides whether a report
		// reads "the agent could not do these" or "the agent ran out of time on
		// these", which are different problems with different fixes.
		//
		// A passing verdict is left alone: the work was done, and how the
		// process ended afterwards does not unmake it.
		if status == contract.StatusFailed && isAgentTimeout(res.ExceptionInfo) {
			return contract.StatusTimedOut, failureClass, errText
		}
		return status, failureClass, errText
	}
	if res.ExceptionInfo == nil {
		return contract.StatusGraderError, "no-verdict",
			"the trial recorded no verifier result and no exception, so nothing judged the attempt"
	}
	return statusForException(res.ExceptionInfo.Type), failureClass, errText
}

// isAgentTimeout reports whether the trial ended because the agent's own
// budget expired, as opposed to the verifier's or the environment's.
func isAgentTimeout(info *ExceptionInfo) bool {
	return info != nil && statusForException(info.Type) == contract.StatusTimedOut
}

// statusForException maps Harbor's failure types onto the trial taxonomy.
//
// Harbor's timeout classes are the interesting ones. The agent's own timeout is
// the task's budget expiring, which is the same fact as a BuildMax wall-time
// limit and a judgement about the subject. A verifier timeout is grading that
// could not finish, so the attempt is unscored rather than failed. An
// environment that would not start is infrastructure and blames neither.
func statusForException(kind string) contract.TrialStatus {
	switch {
	case strings.HasPrefix(kind, "AgentTimeout"), strings.HasPrefix(kind, "AgentSetupTimeout"):
		return contract.StatusTimedOut
	case strings.HasPrefix(kind, "VerifierTimeout"):
		return contract.StatusGraderError
	case strings.HasPrefix(kind, "EnvironmentStartTimeout"):
		return contract.StatusInfrastructureError
	case strings.HasSuffix(kind, "AgentExitCodeError"), strings.HasPrefix(kind, "Api"),
		strings.HasPrefix(kind, "AgentAuthentication"), strings.HasPrefix(kind, "ModelNotFound"):
		return contract.StatusAgentError
	default:
		// Unknown means unknown. Guessing agent_error would let a harness fault
		// this build has not seen count against the subject.
		return contract.StatusInfrastructureError
	}
}

// verifierResult records what the benchmark decided, and reports whether that
// is a verdict at all.
func verifierResult(res TrialResult, pins Pins) (contract.GraderResult, bool) {
	grader := contract.GraderResult{
		Name: VerifierGrader,
		// The verifier is the task's own tests, versioned by the dataset the
		// pin names rather than by a number of ours.
		Version:     pins.Adapter.Version,
		Kind:        contract.GraderDeterministic,
		Required:    true,
		Explanation: "Harbor verifier, dataset " + pins.Dataset.Name + "@" + pins.Dataset.Ref,
	}
	if res.VerifierResult == nil || len(res.VerifierResult.Rewards) == 0 {
		grader.Verdict = contract.VerdictUnknown
		grader.Error = "the trial recorded no verifier reward"
		return grader, false
	}

	reward, ok := singleReward(res.VerifierResult.Rewards)
	if !ok {
		grader.Verdict = contract.VerdictUnknown
		grader.Error = fmt.Sprintf("the trial reported %d rewards and none named %q, so no single verdict follows",
			len(res.VerifierResult.Rewards), rewardKey)
		return grader, false
	}

	score := reward
	grader.Score = &score
	grader.Explanation = fmt.Sprintf("%s = %g", rewardKey, reward)
	if reward >= PassingReward {
		grader.Verdict = contract.VerdictPass
	} else {
		grader.Verdict = contract.VerdictFail
	}
	return grader, true
}

// singleReward picks the one value a verdict follows from. A task reporting a
// single reward decides on it whatever it called it; a task reporting several
// decides on the conventional key, and one doing neither is ambiguous rather
// than passing on whichever entry sorted first.
func singleReward(rewards map[string]float64) (float64, bool) {
	if len(rewards) == 1 {
		for _, v := range rewards {
			return v, true
		}
	}
	if v, ok := rewards[rewardKey]; ok {
		return v, true
	}
	return 0, false
}

func applyUsage(bundle *contract.TrialBundle, res TrialResult, envelope *agentEnvelope, retention contract.RetentionLevel) {
	if envelope != nil {
		bundle.Usage = envelope.usage()
		if retention != contract.RetentionMetadata {
			bundle.Reply = envelope.Reply
		}
		return
	}
	// No envelope: the trial died before BuildMax wrote one, or an older
	// adapter produced it. Harbor's own numbers are second-hand but real.
	if res.AgentResult == nil {
		return
	}
	bundle.Usage = contract.Usage{
		PromptTokens:     intOrZero(res.AgentResult.InputTokens),
		CompletionTokens: intOrZero(res.AgentResult.OutputTokens),
		CacheReadTokens:  intOrZero(res.AgentResult.CacheTokens),
	}
	if res.AgentResult.CostUSD != nil {
		nano := int64(math.Round(*res.AgentResult.CostUSD * nanoUnitsPerUnit))
		bundle.Usage.Cost = &nano
		bundle.Usage.Currency = "USD"
		// Harbor holds cost as a float in dollars, so the integer recovered
		// from it is rounded rather than exact. Saying so beats a sum that
		// silently drifts from what the provider billed.
		bundle.Usage.CostIncomplete = true
	}
}

// applyTrace points the bundle at the durable trace the adapter copied out.
//
// The path is relative to the trial directory rather than copied into the
// bundle: a Harbor job directory is already the evidence store, and duplicating
// gigabytes of traces to restate that would make the import the expensive part.
func applyTrace(bundle *contract.TrialBundle, t Trial, envelope *agentEnvelope) {
	if envelope == nil || envelope.TracePath == "" {
		return
	}
	// The envelope records the path inside the container. The adapter copied
	// the sessions tree out, so the same file is under the trial's agent dir at
	// the same session-relative position.
	idx := strings.Index(envelope.TracePath, "/"+AgentSessionsDir+"/")
	if idx < 0 {
		return
	}
	rel := filepath.FromSlash(strings.TrimPrefix(envelope.TracePath[idx+1:], "/"))
	bundle.TracePath = filepath.Join(TrialAgentDir, rel)

	// The envelope carries tool calls and tokens but not model calls, and
	// reliability reporting needs to tell one expensive call from ten cheap
	// ones. A trace that cannot be read leaves the count at zero rather than
	// failing the import: the verdict is already decided, and losing a
	// diagnostic must not turn a graded attempt into an unreadable one.
	calls, err := trace.CountLLMCalls(filepath.Join(t.Dir, bundle.TracePath))
	if err != nil {
		bundle.Error = appendReason(bundle.Error, "model calls uncounted: "+err.Error())
		return
	}
	bundle.Usage.LLMCalls = calls
}

// appendReason joins a second explanation onto an error string without losing
// the first: an attempt can both fail and have an unreadable trace.
func appendReason(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + "; " + add
}

// reproduceCommand is built by the same code that starts a run, so the command
// filed as evidence is the command that would produce it. They were written
// twice before, and the one nobody executed is the one that drifted: the run
// documented in the README dropped the dataset ref, while this kept it.
func reproduceCommand(t Trial, pins Pins) []string {
	return RunCommand(pins, RunSpec{
		Agent: t.Config.Agent.ImportPath,
		Model: modelTarget(t),
		Tasks: []string{t.Result.TaskName},
		// One attempt: this reproduces a single trial, not the job it came from.
		Attempts: 1,
		Kwargs:   t.Config.Agent.Kwargs,
	})
}

func metadataOf(t Trial) map[string]any {
	if t.Result.AgentResult == nil {
		return nil
	}
	return t.Result.AgentResult.Metadata
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// intField reads a number the adapter recorded. A trial that left it unset
// records null, and JSON carries every number as a float, so both arrive here
// as the zero the manifest then omits — which is what an unstated window is.
func intField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func intOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// sortedKeys orders agent kwargs so a reconstructed command is the same string
// on every import of the same job.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
