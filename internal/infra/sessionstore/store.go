package sessionstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/session"
)

// ErrSessionExists reports that Create was called for a session id that
// already has a journal on disk.
var ErrSessionExists = errors.New("session already exists")

// FileStore is the file-backed implementation of session.Store.
//
// Dir is the sessions root — <BUILDMAX_HOME>/sessions — holding index.json
// directly and one subdirectory per session named by its id.
type FileStore struct {
	Dir string
}

// NewFileStore returns a Store rooted at dir.
func NewFileStore(dir string) *FileStore {
	return &FileStore{Dir: dir}
}

func (s *FileStore) sessionDir(id string) string {
	return filepath.Join(s.Dir, id)
}

// DefaultMeta is what Load, Open, and UpdateMeta fall back to when meta.json
// is damaged: a visible, unpinned user session with zeroed aggregates and no
// title. Reported usage is then wrong, which §5 accepts because it is
// reporting; recovering as visible rather than hidden is what keeps a damaged
// session findable instead of silently lost.
func DefaultMeta(id string) session.Meta {
	return session.Meta{Version: session.MetaVersion, ID: id, Kind: session.KindUser}
}

// Create makes a new session directory with its metadata and an empty,
// headered journal.
//
// The existence check and the journal's own O_EXCL both guard against
// duplicate creation; the check exists so a genuine conflict is reported
// before meta.json is touched, rather than after it has already been
// overwritten out from under an existing session.
func (s *FileStore) Create(ctx context.Context, meta session.Meta) error {
	if err := meta.Validate(); err != nil {
		return err
	}
	dir := s.sessionDir(meta.ID)
	if _, err := os.Stat(filepath.Join(dir, JournalFile)); err == nil {
		return fmt.Errorf("%w: %s", ErrSessionExists, meta.ID)
	}
	if err := WriteMeta(dir, meta); err != nil {
		return err
	}
	j, err := CreateJournal(dir, session.NewHeader(meta.ID, meta.CreatedAt))
	if err != nil {
		return err
	}
	if err := j.Close(); err != nil {
		return err
	}
	if meta.Hidden {
		// Hidden sessions never enter the picker projection; see §9 and §12.
		return nil
	}
	return s.refreshIndexRow(meta)
}

// Open acquires the writer lock, repairs a torn tail, and — if the branch it
// finds has calls left uncertain by an interruption — appends the repair
// records §7.3 describes before returning. The gate is "does anything need
// resolving", not "was the turn left open": a session already repaired by an
// earlier Open still reports an open turn on a second one, but with no
// uncertain calls left, so nothing is appended twice.
func (s *FileStore) Open(ctx context.Context, id string) (_ session.Writer, err error) {
	dir := s.sessionDir(id)
	lock, err := AcquireWriter(filepath.Join(dir, "writer.lock"))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = lock.Release()
		}
	}()

	meta, metaErr := ReadMeta(dir)
	switch {
	case errors.Is(metaErr, ErrMetaNotFound):
		return nil, fmt.Errorf("%w: %s", session.ErrSessionNotFound, id)
	case metaErr != nil:
		meta = DefaultMeta(id)
	}

	j, contents, err := OpenAppend(dir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = j.Close()
		}
	}()
	if err := checkHeaderMatchesDir(contents, id); err != nil {
		return nil, err
	}

	loaded, lastSeq, err := s.openBranch(j, meta, contents.Items)
	if err != nil {
		return nil, err
	}
	return &fileWriter{lock: lock, journal: j, loaded: loaded, lastID: loaded.Head, lastSeq: lastSeq}, nil
}

// openBranch derives Loaded from items and, if repair is needed, performs it
// and re-derives from the extended branch — recomputed rather than patched by
// hand, so the result is always in lockstep with what Reduce and Analyze would
// say about the file as it now stands.
func (s *FileStore) openBranch(j *Journal, meta session.Meta, items []session.Item) (session.Loaded, uint64, error) {
	loaded, err := deriveLoaded(meta, items)
	if err != nil {
		return session.Loaded{}, 0, err
	}
	if rec := loaded.Recovery; len(rec.Uncertain) > 0 {
		repair := buildRecoveryItems(rec, loaded.Head, items[len(items)-1].Seq, time.Now())
		if err := j.Append(repair...); err != nil {
			return session.Loaded{}, 0, err
		}
		items = append(items, repair...)
		loaded, err = deriveLoaded(meta, items)
		if err != nil {
			return session.Loaded{}, 0, err
		}
		// Report what was just repaired, not the now-clean branch: a caller
		// telling the user "this session was interrupted" needs to know what
		// happened, and re-deriving from the repaired items would only ever
		// say there was nothing to repair.
		loaded.Recovery = rec
	}
	var lastSeq uint64
	if len(items) > 0 {
		lastSeq = items[len(items)-1].Seq
	}
	return loaded, lastSeq, nil
}

// buildRecoveryItems is the write §7.3 describes: one turn_recovered record
// naming the interrupted turn, then one tool_result with status unknown per
// uncertain call, chained in sequence starting after parent/seq.
func buildRecoveryItems(rec session.Recovery, parent string, seq uint64, ts time.Time) []session.Item {
	ids := make([]string, 0, len(rec.Uncertain))
	for _, u := range rec.Uncertain {
		ids = append(ids, u.ToolCallID)
	}
	seq++
	head := session.NewItem(seq, session.NewID(), parent, ts, rec.TurnID, session.TurnRecovered{
		TurnID:               rec.TurnID,
		UncertainToolCallIDs: ids,
	})
	items := []session.Item{head}
	parent = head.ID
	for _, u := range rec.Uncertain {
		seq++
		it := session.NewItem(seq, session.NewID(), parent, ts, rec.TurnID, session.ToolResult{
			ToolCallID: u.ToolCallID,
			Status:     session.ToolStatusUnknown,
		})
		items = append(items, it)
		parent = it.ID
	}
	return items
}

// Load reads a session without acquiring the writer lock, repairing a torn
// tail, or writing a recovery record — a writer may be active concurrently,
// and inspection never repairs (§7.2). A branch left mid-turn still reports
// Recovery.Needed() so a caller that only wants to display the session's state
// can see that, without anything being written.
func (s *FileStore) Load(ctx context.Context, id string, mode session.LoadMode) (session.Loaded, error) {
	dir := s.sessionDir(id)
	meta, metaErr := ReadMeta(dir)
	switch {
	case errors.Is(metaErr, ErrMetaNotFound):
		return session.Loaded{}, fmt.Errorf("%w: %s", session.ErrSessionNotFound, id)
	case metaErr != nil:
		meta = DefaultMeta(id)
	}
	if mode == session.LoadMetaOnly {
		return session.Loaded{Meta: meta}, nil
	}

	contents, err := Read(dir)
	if err != nil {
		// If history is damaged, valid metadata may keep the session visible
		// but cannot make it resumable (§5), so a full load surfaces the
		// error rather than silently downgrading to meta-only.
		return session.Loaded{}, err
	}
	if err := checkHeaderMatchesDir(contents, id); err != nil {
		return session.Loaded{}, err
	}
	return deriveLoaded(meta, contents.Items)
}

// deriveLoaded computes Head, State, and Recovery from items. It performs no
// repair; that is layered on separately by openBranch for callers that intend
// to continue the session.
func deriveLoaded(meta session.Meta, items []session.Item) (session.Loaded, error) {
	loaded := session.Loaded{Meta: meta}
	if len(items) == 0 {
		return loaded, nil
	}
	head, err := session.Head(items)
	if err != nil {
		return session.Loaded{}, err
	}
	st, err := session.Reduce(items, head)
	if err != nil {
		return session.Loaded{}, err
	}
	rec, err := session.Analyze(items, head)
	if err != nil {
		return session.Loaded{}, err
	}
	loaded.Head = head
	loaded.Items = items
	loaded.State = st
	loaded.Recovery = rec
	return loaded, nil
}

// checkHeaderMatchesDir rejects a journal whose header names a different
// session than the directory holding it — corruption, not an alternate
// spelling; see §6.1.
func checkHeaderMatchesDir(c Contents, id string) error {
	if c.Header.SessionID != "" && c.Header.SessionID != id {
		return fmt.Errorf("%w: journal header names session %s, directory is %s",
			session.ErrHistoryCorrupt, c.Header.SessionID, id)
	}
	return nil
}

// UpdateMeta changes current selections or running aggregates. It briefly
// takes the writer lock even though it never touches the journal, so a rename
// or pin change cannot race a concurrent update to the same file; a session
// mid-turn reports ErrLocked rather than losing one of the two writes.
func (s *FileStore) UpdateMeta(ctx context.Context, id string, update session.MetaUpdate) error {
	dir := s.sessionDir(id)
	lock, err := AcquireWriter(filepath.Join(dir, "writer.lock"))
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	meta, metaErr := ReadMeta(dir)
	switch {
	case errors.Is(metaErr, ErrMetaNotFound):
		return fmt.Errorf("%w: %s", session.ErrSessionNotFound, id)
	case metaErr != nil:
		meta = DefaultMeta(id)
	}
	updated := session.ApplyMetaUpdate(meta, update, time.Now())
	if err := WriteMeta(dir, updated); err != nil {
		return err
	}
	return s.refreshIndexRow(updated)
}

// List returns the picker projection. includeHidden's two paths cost
// differently: the ordinary picker (false) reads index.json, which never
// holds hidden rows, and never scans; includeHidden is a maintenance path that
// does scan, since there is no cheap projection of what the index deliberately
// excludes.
func (s *FileStore) List(ctx context.Context, includeHidden bool) ([]session.ItemSummary, error) {
	if includeHidden {
		metas, err := scanSessionMetas(s.Dir)
		if err != nil {
			return nil, err
		}
		rows := make([]session.ItemSummary, 0, len(metas))
		for _, m := range metas {
			rows = append(rows, summarize(m))
		}
		return rows, nil
	}
	rows, err := ReadIndex(s.Dir)
	if err != nil {
		return RebuildIndex(s.Dir)
	}
	return rows, nil
}

// refreshIndexRow updates one session's row in index.json without rescanning
// every session. A lost update is possible if two sessions' UpdateMeta calls
// race on this file — the writer lock guards one session's meta.json, not the
// shared index — but the failure mode is a stale picker row for one session
// until it changes again or the index is rebuilt, not data loss: meta.json,
// the row's authority, is unaffected. index.json is a disposable cache by
// design (§4, §12), which is what makes that an acceptable Alpha tradeoff.
func (s *FileStore) refreshIndexRow(m session.Meta) error {
	rows, err := ReadIndex(s.Dir)
	if err != nil {
		_, err := RebuildIndex(s.Dir)
		return err
	}
	if m.Hidden {
		rows = removeRow(rows, m.ID)
	} else {
		rows = upsertRow(rows, summarize(m))
	}
	return WriteIndex(s.Dir, rows)
}

// fileWriter is the FileStore implementation of session.Writer.
type fileWriter struct {
	lock    *WriterLock
	journal *Journal
	loaded  session.Loaded
	lastID  string
	lastSeq uint64
}

func (w *fileWriter) Loaded() session.Loaded { return w.loaded }

// Append validates that items continue exactly from where this Writer's view
// of the branch left off before writing anything. A mismatch is a caller bug,
// not a race: the writer lock already rules out a second writer for the whole
// life of this Writer, so there is nothing here for an optimistic-concurrency
// retry to resolve.
func (w *fileWriter) Append(ctx context.Context, items ...session.Item) error {
	if len(items) == 0 {
		return nil
	}
	parent := w.lastID
	seq := w.lastSeq
	for i, it := range items {
		seq++
		if it.Seq != seq {
			return fmt.Errorf("append: item %d (%s) has seq %d, want %d", i, it.ID, it.Seq, seq)
		}
		if it.ParentID != parent {
			return fmt.Errorf("append: item %d (%s) has parent %q, want %q", i, it.ID, it.ParentID, parent)
		}
		parent = it.ID
	}
	if err := w.journal.Append(items...); err != nil {
		return err
	}
	w.lastID, w.lastSeq = parent, seq
	w.loaded.Items = append(w.loaded.Items, items...)
	w.loaded.Head = parent
	return nil
}

func (w *fileWriter) Close() error {
	err := w.journal.Close()
	lockErr := w.lock.Release()
	if err != nil {
		return err
	}
	return lockErr
}
