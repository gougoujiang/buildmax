package localproject

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gougoujiang/buildmax/internal/util/secretscan"
)

// MemoryVersion is the memory/meta.json format this build writes and reads.
const MemoryVersion = 1

// MaxMemoryChars bounds the whole memory document.
//
// It is not a tuning knob. The document is rendered into every model call and
// has no trimming path, so the ceiling is what keeps always-loaded context to
// roughly a few thousand tokens at worst. It matches the additional
// system-prompt ceiling for the same reason. See
// docs/design/local-project-memory.md §9.1.
const MaxMemoryChars = 8192

var (
	// ErrDigestMismatch reports that the document changed since the caller read
	// it. Nothing is written: the next render shows the newer text, and the
	// caller merges deliberately rather than overwriting another session's
	// update.
	ErrDigestMismatch = errors.New("localproject: project memory changed since it was read")
	// ErrMemoryTooLarge reports a document over MaxMemoryChars.
	ErrMemoryTooLarge = errors.New("localproject: project memory is too large")
	// ErrMemoryNotText reports a document that is not valid UTF-8.
	ErrMemoryNotText = errors.New("localproject: project memory is not valid UTF-8")
	// ErrMemorySecret reports a document holding something that looks like a
	// credential. It is a refusal to persist, not proof of harm, and its
	// absence is not proof of safety.
	ErrMemorySecret = errors.New("localproject: project memory looks like it holds a credential")
)

// Memory is a Project's memory document and what is known about the last write
// to it.
type Memory struct {
	Content string
	Meta    MemoryMeta
	// ManuallyEdited reports that Content does not match the digest metadata
	// records, so a person edited MEMORY.md directly. The Markdown is still the
	// authority -- a read never rewrites a user's file to match stale metadata
	// -- and this only says the provenance below describes an older revision.
	ManuallyEdited bool
}

// MemoryMeta describes the last write BuildMax made. It is not a second copy of
// the content: the Markdown is authoritative, and this exists so a user, a
// diagnostic, and a concurrent writer can each tell revisions apart.
type MemoryMeta struct {
	Version  int    `json:"version"`
	Revision int    `json:"revision"`
	Digest   string `json:"digest,omitempty"`

	UpdatedAt          time.Time `json:"updated_at"`
	UpdatedBySessionID string    `json:"updated_by_session_id,omitempty"`
	UpdatedByRunID     string    `json:"updated_by_run_id,omitempty"`
}

// MemoryWrite is one complete replacement of a Project's memory.
//
// There is no append: replacing the whole document is what makes the agent
// remove what has gone stale instead of adding to a list that only grows.
type MemoryWrite struct {
	Content string
	// ExpectedDigest is the digest of the document the writer was looking at.
	// Empty means "write only if it is still empty" -- the case where no block
	// was rendered because there is no memory yet. It is never an unconditional
	// overwrite: a writer that saw nothing has no basis for discarding what
	// another session has since written.
	ExpectedDigest string

	SessionID string
	RunID     string
}

// MemoryDigest is the content's identity for optimistic concurrency.
//
// An empty document has no digest. That keeps one representation of "there is
// nothing here" across the render, the tool argument, and the stored metadata,
// rather than a hash that a caller would have to know to recognize as the empty
// one.
func MemoryDigest(content string) string {
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ValidateMemory rejects a document this build will not persist or render.
//
// Size is enforced here, on write, rather than by truncating at render time: a
// prefix of a document is not a smaller version of it, and silently sending one
// can change what it says.
func ValidateMemory(content string) error {
	if !utf8.ValidString(content) {
		return ErrMemoryNotText
	}
	if n := utf8.RuneCountInString(content); n > MaxMemoryChars {
		return fmt.Errorf("%w: %d characters, limit %d", ErrMemoryTooLarge, n, MaxMemoryChars)
	}
	return nil
}

// ScanMemoryForSecrets refuses a document holding a recognizable credential.
//
// Best effort, and the agent's own contract -- do not persist credentials or
// surprising sensitive content -- remains the real guard. This catches the
// obvious copy-paste; it proves nothing about what it did not match. The error
// names the shapes, never the values, so a refusal does not put the credential
// into a log or back into the model's context.
func ScanMemoryForSecrets(content string) error {
	if found := secretscan.Findings(content); len(found) > 0 {
		return fmt.Errorf("%w: %s", ErrMemorySecret, strings.Join(found, ", "))
	}
	return nil
}

// NextMemoryMeta is the metadata recorded for a committed write.
func NextMemoryMeta(previous MemoryMeta, w MemoryWrite, now time.Time) MemoryMeta {
	return MemoryMeta{
		Version:            MemoryVersion,
		Revision:           previous.Revision + 1,
		Digest:             MemoryDigest(w.Content),
		UpdatedAt:          now.UTC(),
		UpdatedBySessionID: w.SessionID,
		UpdatedByRunID:     w.RunID,
	}
}
