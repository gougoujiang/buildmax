package desktop

import (
	"context"
	"testing"

	"buildmax/internal/config"
)

func TestApp_Startup_sets_data_dir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BUILDMAX_HOME", dir)
	app := NewApp()
	ctx := context.Background()
	app.Startup(ctx)
	got := config.DataDir()
	if got != dir {
		t.Errorf("DataDir() = %q, want %q", got, dir)
	}
}

func TestApp_Shutdown_noop(t *testing.T) {
	app := NewApp()
	app.Shutdown(context.Background())
}
