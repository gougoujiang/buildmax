package sessionstore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gougoujiang/buildmax/internal/core/session"
)

// JournalFile is the file name inside a session directory.
const JournalFile = "history.jsonl"

// Journal is one session's append-only history file, open for writing.
//
// A Journal does not take the writer lock; the caller does, because ownership
// covers a session's whole mutable state and not one file inside it.
type Journal struct {
	path string
	file *os.File
}

// Contents is a journal read back from disk.
type Contents struct {
	Header session.Header
	Items  []session.Item
	// Truncated is the byte offset a torn final line was cut back to, and is
	// zero when the file ended cleanly. It is reported rather than logged away
	// because losing the tail of an interrupted turn is a fact about the
	// session, not a detail of how it was opened.
	Truncated int64
}

// Create writes a new journal with its header and returns it open for append.
// It fails if one already exists: silently adopting a file would let a bug that
// reuses a session id append one conversation onto another.
func Create(dir string, header session.Header) (*Journal, error) {
	if err := header.Validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, JournalFile)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	j := &Journal{path: path, file: f}
	line, err := json.Marshal(header)
	if err != nil {
		f.Close()
		return nil, err
	}
	if err := j.writeLines([][]byte{line}); err != nil {
		f.Close()
		return nil, err
	}
	return j, nil
}

// Read loads a journal without changing it. A torn final line is reported in
// Contents.Truncated but left on disk: inspection never repairs, so reading a
// session in one window cannot alter what another window is still writing.
func Read(dir string) (Contents, error) {
	return readJournal(filepath.Join(dir, JournalFile), false)
}

// OpenAppend loads a journal, repairs a torn final line, and returns it open for
// append along with what it held. The caller must already own the writer lock;
// repairing a file another process is appending to would cut off its work.
func OpenAppend(dir string) (*Journal, Contents, error) {
	path := filepath.Join(dir, JournalFile)
	contents, err := readJournal(path, true)
	if err != nil {
		return nil, Contents{}, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, Contents{}, err
	}
	return &Journal{path: path, file: f}, contents, nil
}

// Append writes items and returns only once they are durable.
//
// Nothing that affects resume may become visible to the agent before the
// journal has taken it, so a failure here stops the turn rather than being
// logged and passed over. See docs/design/local-session-storage.md §7.1.
func (j *Journal) Append(items ...session.Item) error {
	if len(items) == 0 {
		return nil
	}
	// One record per line needs no escaping pass of its own: JSON encodes a
	// newline inside a string as \n, and compacts raw sub-documents, so a
	// record's bytes can never contain a literal one.
	lines := make([][]byte, 0, len(items))
	for _, it := range items {
		line, err := json.Marshal(it)
		if err != nil {
			return fmt.Errorf("encode item %s: %w", it.ID, err)
		}
		lines = append(lines, line)
	}
	return j.writeLines(lines)
}

// writeLines appends every line in one write, then syncs.
//
// One write keeps a batch from being interleaved with another process's bytes
// on a filesystem that does not honour O_APPEND atomically for large writes, and
// makes a torn tail cut at most one batch rather than scattering it.
func (j *Journal) writeLines(lines [][]byte) error {
	var buf bytes.Buffer
	for _, line := range lines {
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if _, err := j.file.Write(buf.Bytes()); err != nil {
		return err
	}
	// os.File.Sync maps to the strongest primitive each platform offers,
	// F_FULLFSYNC included, which is what the tool-boundary guarantee rests on.
	return j.file.Sync()
}

// Close releases the file. The journal is durable at every Append, so closing
// carries no unwritten state.
func (j *Journal) Close() error {
	if j == nil || j.file == nil {
		return nil
	}
	err := j.file.Close()
	j.file = nil
	return err
}

// Path is the journal's location, for errors a person has to act on.
func (j *Journal) Path() string { return j.path }

// Salvage reads as much of a corrupt journal as is trustworthy: every record up
// to the first one that fails, and no further.
//
// It never writes. A journal that can only be reported as broken, with no path
// to the work inside it, is a worse outcome than a conservative refusal plus a
// way to recover the part that is intact.
func Salvage(dir string) (Contents, error) {
	contents, err := readJournal(filepath.Join(dir, JournalFile), false)
	if err == nil {
		return contents, nil
	}
	var stop stopAt
	if !errors.As(err, &stop) {
		return Contents{}, err
	}
	return stop.contents, nil
}

// stopAt carries the prefix that was still valid when a load failed, so Salvage
// can offer it without parsing the file a second time.
type stopAt struct {
	contents Contents
	err      error
}

func (s stopAt) Error() string { return s.err.Error() }
func (s stopAt) Unwrap() error { return s.err }

func readJournal(path string, repair bool) (Contents, error) {
	f, err := os.Open(path)
	if err != nil {
		return Contents{}, err
	}
	defer f.Close()

	var out Contents
	reader := bufio.NewReader(f)
	validator := session.NewValidator()
	var offset int64
	for lineNo := 0; ; lineNo++ {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return Contents{}, readErr
		}
		if len(line) > 0 && line[len(line)-1] != '\n' {
			// A final line with no newline is a torn tail: a record is written
			// with its newline last, so a partial write can only end here.
			// Anything else broken is corruption, which is not repaired.
			if lineNo == 0 {
				// The header was never completed, so there is no journal to
				// repair back to — an empty file would claim more than is known.
				return Contents{}, fmt.Errorf("%w: %s has an incomplete header", session.ErrHistoryCorrupt, path)
			}
			if repair {
				if err := truncateTo(path, offset); err != nil {
					return Contents{}, err
				}
				out.Truncated = offset
			}
			return out, finish(validator, out, path)
		}
		if len(line) == 0 {
			if lineNo == 0 {
				return Contents{}, fmt.Errorf("%w: %s is empty", session.ErrHistoryCorrupt, path)
			}
			return out, finish(validator, out, path)
		}
		offset += int64(len(line))
		if lineNo == 0 {
			if err := json.Unmarshal(line, &out.Header); err != nil {
				return Contents{}, fmt.Errorf("%w: %s header: %v", session.ErrHistoryCorrupt, path, err)
			}
			if err := out.Header.Validate(); err != nil {
				return Contents{}, fmt.Errorf("%s: %w", path, err)
			}
			continue
		}
		var item session.Item
		if err := json.Unmarshal(line, &item); err != nil {
			return Contents{}, stopAt{contents: out, err: fmt.Errorf("%w: %s line %d: %v", session.ErrHistoryCorrupt, path, lineNo+1, err)}
		}
		// Validating as each record arrives is what lets a damaged journal name
		// the failing line and still hand back the prefix before it.
		if err := validator.Add(item); err != nil {
			return Contents{}, stopAt{contents: out, err: fmt.Errorf("%s line %d: %w", path, lineNo+1, err)}
		}
		out.Items = append(out.Items, item)
	}
}

// finish applies the checks only a complete journal can make, reporting the file
// the problem is in — the part a person needs and the core package cannot know.
func finish(v *session.Validator, out Contents, path string) error {
	if err := v.Done(); err != nil {
		return stopAt{contents: out, err: fmt.Errorf("%s: %w", path, err)}
	}
	return nil
}

// truncateTo cuts a torn tail back to the last complete record and syncs, so a
// crash during the repair cannot leave a second partial line behind it.
func truncateTo(path string, size int64) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		return err
	}
	return f.Sync()
}
