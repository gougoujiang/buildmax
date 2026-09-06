//go:build unix

package wsarchive

import (
	"os"
	"syscall"
)

// isHardLinked reports whether a regular file has more than one link, which the
// canonical rules forbid: a hard link is a second name for bytes the archive
// also stores under a different path, and restoring it as independent bytes is
// not the tree that was captured.
func isHardLinked(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && uint64(st.Nlink) > 1
}
