//go:build !linux && !darwin

package sandbox

import "errors"

// newBackend returns an error on platforms without an OS-level sandbox
// implementation. The Manager treats this as "backend unavailable" — the
// agent continues with the unsandboxed path unless fail_if_unavailable
// is set. See doc 032 §7 for the platform support matrix.
func newBackend(_ string) (backend, error) {
	return nil, errors.New("sandbox: not supported on this platform")
}
