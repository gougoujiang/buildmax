package artifact

import (
	"context"
	"io"

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
)

// ContentStore holds one immutable content object per artifact.
//
// This service declares the contract it needs; an object-store adapter
// satisfies it structurally, so the storage package never has to know a service
// exists. The reference type is core/artifact's rather than either package's,
// because both have to name it and neither may own it.
//
// Write-once by construction: there is no update, because a changed file is a
// new artifact. The key an implementation derives is private to it — nothing
// here may build, parse, or reconstruct one, which is what leaves the bucket
// layout free to change. See docs/design/unified-artifacts.md section 9.
type ContentStore interface {
	// PutArtifact streams content in under a reserved, unused artifact ID and
	// reports the key it wrote. The key is returned so the record can keep what
	// was actually used rather than a recomputation that a later layout change
	// would silently invalidate; it is for operators reading the database, and
	// never leaves the server.
	PutArtifact(ctx context.Context, ref coreartifact.Ref, r io.Reader) (string, error)
	// OpenArtifact returns the content stream, which the caller closes.
	// apierr.ErrNotFound when the object is absent.
	OpenArtifact(ctx context.Context, ref coreartifact.Ref) (io.ReadCloser, error)
	// RemoveArtifact deletes the object. Content that is already gone is not an
	// error: removal is what the caller asked for and it is already true, and
	// the crash-recovery path must be able to run twice.
	RemoveArtifact(ctx context.Context, ref coreartifact.Ref) error
}
