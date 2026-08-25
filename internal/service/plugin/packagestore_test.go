package plugin

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/mock"
)

// TestAdaptersSatisfyPackageStore keeps every implementation answering the
// contract this package declares. The assertion lives here rather than beside
// the adapters because the port is this service's: an adapter that drifts from
// it has broken somebody else's contract, not its own.
//
// This is the one place plugin code names an object-store package, and it is a
// test. The production path takes whatever bootstrap hands it.
func TestAdaptersSatisfyPackageStore(t *testing.T) {
	var _ PackageStore = (*objectstore.LocalFSPluginPackageStorage)(nil)
	var _ PackageStore = (*objectstore.S3PluginPackageStorage)(nil)
	var _ PackageStore = (*mock.MockPluginPackageStorage)(nil)
}
