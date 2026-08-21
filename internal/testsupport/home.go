package testsupport

import (
	"fmt"
	"os"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

// RunWithIsolatedHome runs a package's tests against a throwaway BUILDMAX_HOME
// and removes it afterwards.
//
// config.DataDir refuses to fall back to the real ~/.buildmax under `go test`,
// so a package whose code reaches it needs a home of its own. Setting it for the
// whole test binary is what `./make test` already does globally; doing it here
// too means a narrow `go test ./internal/x` behaves the same, and a test added
// later does not have to know the requirement exists.
func RunWithIsolatedHome(m *testing.M) int {
	dir, err := os.MkdirTemp("", "buildmax-test-home")
	if err != nil {
		fmt.Fprintf(os.Stderr, "isolate BUILDMAX_HOME: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.Setenv(config.EnvKeyBuildmaxHome, dir); err != nil {
		fmt.Fprintf(os.Stderr, "isolate BUILDMAX_HOME: %v\n", err)
		return 1
	}
	return m.Run()
}
