//go:build !unix

package wsarchive

import "os"

// isHardLinked cannot be answered from os.FileInfo off Unix, and the worker runs
// on Linux; a non-Unix build treats every regular file as unlinked.
func isHardLinked(os.FileInfo) bool { return false }
