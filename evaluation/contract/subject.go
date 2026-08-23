package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// SubjectManifest freezes the configuration a trial measured. Section 2.2 is
// the reason it exists: a run's behavior comes from the revision, the model,
// the instructions, the extensions, the policy, and the host together, so a
// result naming only a model cannot support a qualification or a regression
// decision.
//
// Secrets and private instruction bodies do not belong here. The manifest
// carries their digests and safe provenance instead, so identifying a subject
// never turns a stored result into a credential or a content store.
type SubjectManifest struct {
	ContractVersion int    `json:"contract_version"`
	Name            string `json:"name"`
	// ID is the digest of every other field, from Digest. Two trials share a
	// subject when they share this value, which is what makes a paired
	// comparison honest: the alternative, comparing by name, silently pairs
	// runs whose configuration drifted between them.
	ID string `json:"id"`

	Build        BuildIdentity     `json:"build"`
	Execution    ExecutionIdentity `json:"execution"`
	Model        ModelIdentity     `json:"model"`
	Instructions InstructionInputs `json:"instructions"`
	Extensions   ExtensionInputs   `json:"extensions"`
	Policy       PolicyResolution  `json:"policy"`
	Host         HostProfile       `json:"host"`
	// Dataset is the task collection version this subject was measured over.
	// It sits on the subject as well as the experiment because a bundle read on
	// its own must still say what it was asked.
	Dataset DatasetRef `json:"dataset"`
}

// BuildIdentity is the artifact under evaluation.
type BuildIdentity struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	// Dirty is the digest of uncommitted changes, empty for a clean tree. A
	// boolean would say a local build is unreproducible without saying which
	// local build it was, and two dirty candidates would then compare as the
	// same subject.
	Dirty string `json:"dirty,omitempty"`
	// ArtifactDigest is the binary or container image measured. It is what
	// makes this a black-box result: without it the manifest describes a source
	// revision rather than the thing that ran.
	ArtifactDigest string `json:"artifact_digest"`
}

// ExecutionIdentity is how the trial reached the subject.
type ExecutionIdentity struct {
	Surface Surface `json:"surface"`
	// AdapterVersion changes when the adapter changes how it invokes the
	// subject. An adapter change moves results without the product moving, and
	// a comparison spanning one is not paired.
	AdapterVersion int `json:"adapter_version"`
}

// ModelIdentity is the inference configuration.
type ModelIdentity struct {
	// Transport is how inference was reached: a provider protocol, or the
	// managed gateway. Two subjects reaching the same model through different
	// transports are different subjects.
	Transport string `json:"transport"`
	Target    string `json:"target"`
	Alias     string `json:"alias,omitempty"`
	// Revision is the exact model build the provider reported. Absent means the
	// provider reported none, which the manifest states rather than filling in
	// from the target name: section 8.2 requires recorded uncertainty over
	// invented precision, because a qualification that names a revision the
	// provider never confirmed cannot be re-run against it.
	Revision      string `json:"revision,omitempty"`
	Reasoning     string `json:"reasoning,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
	MaxOutput     int    `json:"max_output,omitempty"`
}

// InstructionInputs identifies what the subject was told, by digest.
type InstructionInputs struct {
	SystemPromptDigest string `json:"system_prompt_digest"`
	ToolSchemaDigest   string `json:"tool_schema_digest"`
	// Layers names the instruction sources in order, without their bodies:
	// a workspace AGENTS.md, an agent definition, session notes. The names are
	// provenance; the digest above is what actually pins the content.
	Layers []string `json:"layers,omitempty"`
}

// ExtensionInputs identifies what the subject loaded from outside itself.
// Section 18.2 records that the trace names plugins but not MCP servers and
// skills separately; the manifest names all three, so a subject stays
// identifiable while that trace gap is open.
type ExtensionInputs struct {
	Plugins  []string `json:"plugins,omitempty"`
	Skills   []string `json:"skills,omitempty"`
	MCP      []string `json:"mcp,omitempty"`
	Subagent []string `json:"subagents,omitempty"`
}

// PolicyResolution is the boundary the subject ran under, as resolved rather
// than as configured. What a settings file requested and what the runtime
// granted differ, and only the second describes the trial.
type PolicyResolution struct {
	Permissions string `json:"permissions"`
	// Sandboxed is a pointer so an unsandboxed subject records false rather
	// than omitting the field, for the reason the trace does the same: an
	// unreported boundary is indistinguishable from an unresolved one, and a
	// reader breaking that tie favourably would credit a subject with
	// protection it never had.
	Sandboxed   *bool    `json:"sandboxed"`
	SandboxMode string   `json:"sandbox_mode,omitempty"`
	Hooks       []string `json:"hooks,omitempty"`
}

// HostProfile is the machine the trial ran on. It separates a regression from
// a slower machine, which section 12 needs before a latency delta means
// anything.
type HostProfile struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	CPUs     int    `json:"cpus,omitempty"`
	MemoryMB int    `json:"memory_mb,omitempty"`
	// Network says what the trial could reach: "none", "proxied", "open". A
	// task failing for lack of network is not an Agent that cannot do it.
	Network string `json:"network,omitempty"`
}

// DatasetRef pins the task collection by immutable version and digest, which is
// what lets a private or rotating holdout use the same contract as the public
// suite without the public suite depending on private access.
type DatasetRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// Digest returns the subject's content identity: the SHA-256 of the manifest
// with ID cleared. Callers set ID from it before recording a trial.
func (m SubjectManifest) Digest() (string, error) {
	m.ID = ""
	encoded, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("digest subject: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// WithID returns the manifest carrying its own digest as ID.
func (m SubjectManifest) WithID() (SubjectManifest, error) {
	id, err := m.Digest()
	if err != nil {
		return SubjectManifest{}, err
	}
	m.ID = id
	return m, nil
}
