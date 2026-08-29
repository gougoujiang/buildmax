package localproject

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gougoujiang/buildmax/internal/util/secretscan"
)

// One memory is one file, and an index over them is what a run carries.
//
// The split is the whole design. The index is resident -- rendered after the
// message list on every call of every session in the Project -- so it is
// bounded by the number of memories and the length of a description. Bodies are
// a retrieval cost paid only when read, so one can afford the Why and the How
// that make it actionable instead of being compressed into a bullet that
// survives the budget by dropping its reason. See
// docs/design/local-project-memory.md §8.3 and §9.1.

// Budgets. Each is enforced on write; none truncates silently at render time.
const (
	// MaxMemories bounds the store. It is a starting bound chosen to be raised
	// on evidence rather than lowered after users have filled it.
	MaxMemories = 20
	// MaxDescriptionChars bounds one index line's payload.
	MaxDescriptionChars = 100
	// MaxBodyChars bounds one body. Generous because it is not resident.
	MaxBodyChars = 2000
	// MaxMemoryNameChars bounds a slug, which is also a file name.
	MaxMemoryNameChars = 64
)

// MemoryType says what kind of knowledge a memory holds.
//
// There is deliberately no user type. Who the user is would be global user
// memory, which §5.2 keeps out of a Project-scoped store.
type MemoryType string

const (
	// MemoryTypeFeedback is guidance the user gave about how to work in this
	// Project. It records what the user wants, never what the user is.
	MemoryTypeFeedback MemoryType = "feedback"
	// MemoryTypeProject is ongoing work, goals, decisions, and constraints not
	// derivable from the code or its history.
	MemoryTypeProject MemoryType = "project"
	// MemoryTypeReference points at external resources: dashboards, tickets,
	// specifications.
	MemoryTypeReference MemoryType = "reference"
)

// MemoryTypes returns every valid type, in the order a surface should list
// them. It returns a fresh slice so no caller can reorder or extend the set the
// validator trusts.
func MemoryTypes() []MemoryType {
	return []MemoryType{MemoryTypeFeedback, MemoryTypeProject, MemoryTypeReference}
}

var (
	// ErrMemoryNotFound reports that no memory has that name.
	ErrMemoryNotFound = errors.New("localproject: no such memory")
	// ErrMemoryUnread reports an attempt to replace a memory this run has not
	// read. It is not a conflict: there is nothing to merge against, because
	// the writer has not seen what it would be overwriting.
	ErrMemoryUnread = errors.New("localproject: memory was not read by this run")
	// ErrMemoryConflict reports that the body changed since this run read it --
	// another session's write, or a direct user edit.
	ErrMemoryConflict = errors.New("localproject: memory changed since this run read it")
	// ErrMemoryFull reports that the store already holds MaxMemories.
	ErrMemoryFull = errors.New("localproject: project memory is full")
	// ErrMemoryInvalid reports a memory this build will not persist.
	ErrMemoryInvalid = errors.New("localproject: invalid memory")
	// ErrMemorySecret reports a body holding something that looks like a
	// credential. It is a refusal to persist, not proof of harm, and its
	// absence is not proof of safety.
	ErrMemorySecret = errors.New("localproject: memory looks like it holds a credential")
)

// Memory is one remembered thing: its own file, its own provenance.
//
// Name is the slug, the file name, and the identity all at once -- there is no
// second identifier that can disagree with it.
type Memory struct {
	Name        string
	Description string
	Type        MemoryType
	// SessionID and UpdatedAt are this memory's own provenance, which is why
	// there is no sidecar metadata file: a lone Markdown document needed one
	// because it had nowhere to record who wrote it, and per-file frontmatter
	// removes that need rather than moving it.
	SessionID string
	UpdatedAt time.Time
	// VerifiedAt is the date a memory that caches something expensive was last
	// checked against its source of truth. Nil for the ordinary memory that
	// asserts nothing it does not itself hold. Only the date is meaningful.
	VerifiedAt *time.Time
	Body       string
}

// SkippedMemory is a file the store could not use. It is reported rather than
// guessed at: skipping one memory is a smaller failure than rendering an index
// line promising a body the read tool cannot return.
type SkippedMemory struct {
	File   string
	Reason string
}

// MemorySet is what the store found in one Project's memory directory.
type MemorySet struct {
	// Memories are the usable ones, ordered by name so the index is stable.
	Memories []Memory
	Skipped  []SkippedMemory
}

// Find returns the memory with that name.
func (s MemorySet) Find(name string) (Memory, bool) {
	for _, m := range s.Memories {
		if m.Name == name {
			return m, true
		}
	}
	return Memory{}, false
}

// MemoryWrite creates or replaces exactly one memory.
//
// PriorDigest is supplied by the runtime from the bodies it has handed the
// model this run, never by the model. Routing a correctness token out to the
// least reliable component in the loop and expecting it back verbatim would add
// an omitted-parameter case whose only plausible fallback is an unconditional
// overwrite. Empty means this run has not read the memory, which is a refusal
// on replacement and irrelevant on creation.
type MemoryWrite struct {
	Name        string
	Description string
	Type        MemoryType
	Body        string
	SessionID   string
	PriorDigest string
}

// BodyDigest identifies a body for the read-then-replace rule.
func BodyDigest(body string) string {
	sum := sha256.Sum256([]byte(body))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ValidMemoryName reports whether s is a usable slug: lowercase letters,
// digits, and single hyphens between them.
//
// The slug is a file name, so the character set is a containment guard as much
// as a convention -- a name that could traverse a directory or collide under a
// case-insensitive filesystem is one the store must never be asked to write.
func ValidMemoryName(s string) bool {
	if s == "" || len(s) > MaxMemoryNameChars {
		return false
	}
	if strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") || strings.Contains(s, "--") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

// ValidMemoryType reports whether t is one this build understands.
func ValidMemoryType(t MemoryType) bool {
	return slices.Contains(MemoryTypes(), t)
}

// Validate rejects a memory this build will not persist or render.
func (m Memory) Validate() error {
	if !ValidMemoryName(m.Name) {
		return fmt.Errorf("%w: name %q must be lowercase letters, digits, and single hyphens, at most %d characters",
			ErrMemoryInvalid, m.Name, MaxMemoryNameChars)
	}
	if !ValidMemoryType(m.Type) {
		return fmt.Errorf("%w: type %q must be one of %s", ErrMemoryInvalid, m.Type, joinTypes())
	}
	desc := strings.TrimSpace(m.Description)
	if desc == "" {
		return fmt.Errorf("%w: %s has no description, and a description is the whole index line",
			ErrMemoryInvalid, m.Name)
	}
	if strings.ContainsAny(desc, "\n\r") {
		return fmt.Errorf("%w: %s has a multi-line description; an index line is one line", ErrMemoryInvalid, m.Name)
	}
	if n := utf8.RuneCountInString(desc); n > MaxDescriptionChars {
		return fmt.Errorf("%w: %s has a %d-character description, limit %d",
			ErrMemoryInvalid, m.Name, n, MaxDescriptionChars)
	}
	if !utf8.ValidString(m.Body) {
		return fmt.Errorf("%w: %s has a body that is not valid UTF-8", ErrMemoryInvalid, m.Name)
	}
	if strings.TrimSpace(m.Body) == "" {
		return fmt.Errorf("%w: %s has an empty body; an empty write is a delete, not a memory",
			ErrMemoryInvalid, m.Name)
	}
	if n := utf8.RuneCountInString(m.Body); n > MaxBodyChars {
		return fmt.Errorf("%w: %s has a %d-character body, limit %d", ErrMemoryInvalid, m.Name, n, MaxBodyChars)
	}
	return nil
}

func joinTypes() string {
	types := MemoryTypes()
	names := make([]string, 0, len(types))
	for _, t := range types {
		names = append(names, string(t))
	}
	return strings.Join(names, ", ")
}

// ScanMemoryForSecrets refuses a body holding a recognizable credential.
//
// Best effort, and the Agent's own contract -- do not persist credentials or
// surprising sensitive information -- remains the real guard. The error names
// the shapes, never the values, so a refusal does not put the credential into a
// log or back into the model's context.
func ScanMemoryForSecrets(body string) error {
	if found := secretscan.Findings(body); len(found) > 0 {
		return fmt.Errorf("%w: %s", ErrMemorySecret, strings.Join(found, ", "))
	}
	return nil
}

// SortMemories orders a set by name, which is what makes the generated index
// stable across writes.
func SortMemories(memories []Memory) {
	sort.Slice(memories, func(i, j int) bool { return memories[i].Name < memories[j].Name })
}
