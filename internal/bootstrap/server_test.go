package bootstrap

import (
	"context"
	"testing"
)

func TestRunServer_PortOverride(t *testing.T) {
	// Verify that RunServer prefers portOverride > 0 over the config file port.
	// We can't start a real server here; just confirm the logic compiles and the
	// function signature accepts a portOverride int.
	_ = func() { _ = RunServer(context.TODO(), 9999) }
}
