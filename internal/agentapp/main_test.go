package agentapp

import (
	"os"
	"testing"

	"github.com/gougoujiang/buildmax/internal/testsupport"
)

// Code in this package reads BUILDMAX_HOME-relative paths, so the whole binary
// runs against a throwaway home rather than the contributor's real one.
func TestMain(m *testing.M) {
	os.Exit(testsupport.RunWithIsolatedHome(m))
}
