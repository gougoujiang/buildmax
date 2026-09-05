package architecture_test

// The worker control API is served on its own listener, off the public HTTP
// surface. That boundary is only real if the two route sets stay disjoint, and
// the composition keeps them so: server.New registers RegisterPublic (every
// package except worker) on the public mux and RegisterWorker (the worker
// package alone) on the worker mux. This test enforces the invariant those two
// methods rely on at the registration source, so a worker route added to the
// wrong package — or a public route sneaking a /api/worker/ pattern — fails
// here rather than surfacing on the public listener. The runtime half is
// TestListenerBoundary in internal/server.
//
// See docs/design/worker-api-network-boundary.md §5.1.

import (
	"path/filepath"
	"strings"
	"testing"
)

const (
	workerRoutePrefix = "/api/worker/"
	workerPackageDir  = "internal/server/handlers/worker"
)

func TestWorkerRoutesAreIsolatedToTheWorkerPackage(t *testing.T) {
	root := repoRoot(t)

	for route, file := range registeredRoutes(t, root) {
		pattern := strings.SplitN(route, " ", 2)[1]
		inWorkerPkg := strings.HasPrefix(filepath.ToSlash(file), workerPackageDir+"/")
		isWorkerRoute := strings.HasPrefix(pattern, workerRoutePrefix)

		switch {
		case isWorkerRoute && !inWorkerPkg:
			t.Errorf("%s is a worker route but is registered in %s; worker routes belong to %s so RegisterPublic never puts them on the public listener",
				route, file, workerPackageDir)
		case inWorkerPkg && !isWorkerRoute:
			t.Errorf("%s is registered in the worker package (%s) but is not under %s; the worker listener serves only worker routes",
				route, file, workerRoutePrefix)
		}
	}
}
