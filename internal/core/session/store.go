package session

import (
	"context"
	"time"
)

// LoadMode controls how much of a session Store.Load reads.
type LoadMode int

const (
	// LoadMetaOnly reads meta.json alone, for listing and presentation paths
	// that do not need the journal.
	LoadMetaOnly LoadMode = iota
	// LoadFull replays the branch ending at the current head and validates it.
	LoadFull
)

// Loaded is what Store.Load or Store.Open returns.
type Loaded struct {
	Meta Meta
	// Head is the current head's id, or "" for a session with no items yet.
	Head string
	// Items is the branch from the root to Head, in logical order. Populated
	// only for LoadFull.
	Items []Item
	// State is Reduce(Items, Head). Populated only for LoadFull.
	State State
	// Recovery classifies an interrupted turn on this branch, per §7.3.
	// Populated only for LoadFull.
	Recovery Recovery
}

// ItemSummary is one session's row in the picker projection (§12): enough to
// list and group forks without reading a session's journal.
type ItemSummary struct {
	ID         string
	Kind       Kind
	Title      string
	Workspace  string
	Pinned     bool
	UpdatedAt  time.Time
	ForkedFrom *ForkedFrom
}

// Store is the persistence seam between AgentApp and physical storage. It
// expresses session semantics, not paths: core owns what these operations
// mean, infra owns making them durable. See
// docs/design/local-session-storage.md §14.
//
// Store itself never appends to a journal. Appending requires the writer lock,
// which Open acquires and Writer.Close releases — a capability the interface
// makes explicit so a caller cannot append without holding it, and so the lock
// is held for as long as a caller is actively committing a turn rather than
// re-acquired call by call, which is what keeps two turns from interleaving
// into one open span. See §12.
type Store interface {
	// Create makes a new session directory with its metadata and an empty,
	// headered journal. It fails if the session already exists.
	Create(ctx context.Context, meta Meta) error

	// Open acquires the writer lock and returns a Writer for id, along with
	// what was already on this session's journal — repairing a torn tail and
	// classifying an interrupted turn for recovery in the process. It fails
	// with ErrLocked if another process already holds the lock.
	Open(ctx context.Context, id string) (Writer, error)

	// Load reads a session without acquiring the writer lock or repairing
	// anything. A writer may be active concurrently; Load only ever sees a
	// stable prefix.
	Load(ctx context.Context, id string, mode LoadMode) (Loaded, error)

	// UpdateMeta changes current selections or running aggregates. It cannot
	// change anything resumable — MetaUpdate has no field for that — so it
	// never touches the journal.
	UpdateMeta(ctx context.Context, id string, update MetaUpdate) error

	// List returns the picker projection. includeHidden controls whether
	// subagent sessions (§9) are included; the ordinary picker passes false.
	List(ctx context.Context, includeHidden bool) ([]ItemSummary, error)
}

// Writer is one session held open for append. It owns the writer lock for its
// whole lifetime: nothing else may append to, rewind, or fork from this
// session until Close releases it.
type Writer interface {
	// Loaded is what Open found when the writer lock was acquired.
	Loaded() Loaded

	// Append writes items and returns only once they are durable. items must
	// continue the branch this Writer opened: each item's ParentID must chain
	// from Loaded().Head (or from a preceding item in the same call), and
	// their Seq values must be contiguous starting after the highest Seq this
	// Writer has appended so far. A caller that gets this wrong is a bug, not
	// a race — the lock already rules out a second writer — so Append reports
	// it as an ordinary error rather than needing an optimistic-concurrency
	// retry.
	Append(ctx context.Context, items ...Item) error

	// Close releases the writer lock. It is always safe to call, including
	// after Append has failed.
	Close() error
}
