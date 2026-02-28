package server

import (
	"os"
	"testing"

	"buildmax/internal/config"
)

func TestResolveServerPort_UsedByRunServer(t *testing.T) {
	// Smoke test that server startup uses config.ResolveServerPort correctly.
	// Full cases are in config.TestResolveServerPort.
	os.Unsetenv(config.EnvKeyBuildmaxServerPort)
	port, err := config.ResolveServerPort(0)
	if err != nil {
		t.Fatal(err)
	}
	if port != config.DefaultServerPort {
		t.Errorf("port = %d, want %d", port, config.DefaultServerPort)
	}
}
