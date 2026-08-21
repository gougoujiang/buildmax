package bootstrap

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	buildmaxlog "github.com/gougoujiang/buildmax/internal/infra/log"
)

// A worker process runs one task run, so RunWorker stamps the run onto the
// default logger before anything can fail. Everything the worker drives -- the
// agent loop, the LLM client, the tools -- logs through that default and could
// not be handed the run id any other way.
func TestWorkerStampsItsRunOnEveryRecord(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxHome, t.TempDir())
	buildmaxlog.Init(buildmaxlog.LogConfig{LogsDir: t.TempDir(), Level: "debug"})
	var buf bytes.Buffer
	buildmaxlog.SetOutput(&buf)
	restore := slog.Default()
	t.Cleanup(func() { slog.SetDefault(restore) })

	// No server.yaml, so this fails early. That is the point: identity has to be
	// set before the first thing that can go wrong.
	_ = RunWorker(context.Background(), "r_logging")

	// A package that knows nothing about the run still logs it.
	slog.Info("from somewhere else entirely")

	out := buf.String()
	for _, want := range []string{"task_run_id=r_logging", "component=worker", "from somewhere else entirely"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in %q", want, out)
		}
	}
	if strings.Count(out, "task_run_id=r_logging") < 2 {
		t.Errorf("the run id should be on the worker's own records and on later ones: %q", out)
	}
}

func TestWorkerRefusesAnEmptyRunID(t *testing.T) {
	if err := RunWorker(context.Background(), ""); err == nil {
		t.Fatal("want an error for an empty task-run-id")
	}
}
