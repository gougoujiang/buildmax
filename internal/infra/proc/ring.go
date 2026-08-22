package proc

import "sync"

// OutputChunk is one incremental read of captured output. Offsets are
// absolute over the stream's lifetime, so a cursor survives the ring
// dropping old bytes — the reader learns how much it missed instead of
// silently rereading shifted data.
type OutputChunk struct {
	Data []byte
	// Next is the cursor for the following read.
	Next uint64
	// Dropped counts bytes between the requested cursor and Data[0] that are
	// no longer retained.
	Dropped uint64
}

// ring is a bounded byte buffer keeping the newest bytes. Write never blocks
// the producing pipe copy; overflow discards the oldest bytes.
type ring struct {
	mu    sync.Mutex
	buf   []byte
	limit int
	start uint64 // absolute offset of buf[0]
}

func newRing(limit int) *ring { return &ring{limit: limit} }

// Write implements io.Writer for exec's pipe copies. It always reports full
// success: a bounded ring absorbs any volume by forgetting the oldest bytes.
func (r *ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(p) >= r.limit {
		r.start += uint64(len(r.buf)) + uint64(len(p)-r.limit)
		r.buf = append(r.buf[:0], p[len(p)-r.limit:]...)
		return len(p), nil
	}
	if over := len(r.buf) + len(p) - r.limit; over > 0 {
		r.start += uint64(over)
		r.buf = append(r.buf[:0], r.buf[over:]...)
	}
	r.buf = append(r.buf, p...)
	return len(p), nil
}

// read returns a copy of retained bytes from cursor, at most max (<= 0 means
// all). After a drop, leading UTF-8 continuation bytes are skipped so the
// chunk does not begin mid-rune; the skipped bytes count as dropped.
func (r *ring) read(cursor uint64, max int) OutputChunk {
	r.mu.Lock()
	defer r.mu.Unlock()
	chunk := OutputChunk{Next: cursor}
	if cursor < r.start {
		chunk.Dropped = r.start - cursor
		cursor = r.start
	}
	end := r.start + uint64(len(r.buf))
	if cursor > end {
		chunk.Next = end
		return chunk
	}
	data := r.buf[cursor-r.start:]
	if chunk.Dropped > 0 {
		for len(data) > 0 && data[0]&0xC0 == 0x80 {
			data = data[1:]
			cursor++
			chunk.Dropped++
		}
	}
	if max > 0 && len(data) > max {
		data = data[:max]
	}
	chunk.Data = append([]byte(nil), data...)
	chunk.Next = cursor + uint64(len(data))
	return chunk
}
