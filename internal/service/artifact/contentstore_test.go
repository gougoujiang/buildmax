package artifact

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/mock"
)

// TestAdaptersSatisfyContentStore keeps every implementation answering the
// contract this package declares. The assertion lives here rather than beside
// the adapters because the port is this service's: an adapter that drifts from
// it has broken somebody else's contract, not its own.
//
// This is the one place artifact code names an object-store package, and it is
// a test. The production path takes whatever bootstrap hands it.
func TestAdaptersSatisfyContentStore(t *testing.T) {
	var _ ContentStore = (*objectstore.LocalFSArtifactStorage)(nil)
	var _ ContentStore = (*objectstore.S3ArtifactStorage)(nil)
	var _ ContentStore = (*mock.MockArtifactStorage)(nil)
}
