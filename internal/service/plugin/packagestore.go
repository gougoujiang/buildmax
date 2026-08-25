package plugin

import (
	"context"
	"io"
)

// PackageStore holds the immutable bytes of published releases.
//
// This service declares the contract it needs; an object-store adapter
// satisfies it structurally, so the storage package never has to know a service
// exists. Only the two implementations under internal/infra/objectstore and the
// in-memory test double implement it.
//
// It is deliberately not artifact storage. A package belongs to the
// deployment's catalog rather than to a team's run, so it must not inherit team
// artifact authorization or retention — a catalog entry that vanished with a
// team's retention window could no longer explain an installation that is still
// on somebody's machine.
//
// Every method streams. A package is bounded, but bounded at tens of megabytes,
// and a server that read one into memory per request would be sized by its
// largest plugin rather than by its traffic.
type PackageStore interface {
	// PackageKey returns the object key one release's bytes are stored under.
	//
	// The layout belongs to the implementation; what this service does with the
	// result is persist it on the release record and hand it back to Open. It
	// also validates: a plugin name that is not one path segment, or a digest
	// that is not a lower-case sha256, has no key.
	PackageKey(prefix, pluginName, digest string) (string, error)
	// Put stores bytes under key. Keys are content-addressed, so writing one
	// twice writes the same bytes and a partial write is never visible.
	Put(ctx context.Context, key string, r io.Reader) error
	// Open returns the object and its size. A missing object is reported as the
	// storage layer's not-found error, which the plugin HTTP handler maps.
	Open(ctx context.Context, key string) (io.ReadCloser, int64, error)
	// Exists reports whether the bytes are already stored, so a republish of
	// identical content does not have to upload them again.
	Exists(ctx context.Context, key string) (bool, error)
}
