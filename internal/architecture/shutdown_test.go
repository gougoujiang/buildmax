package architecture_test

// Shutdown constraints. An orderly stop depends on two things no compiler
// checks: every streaming handler having decided whether it ends when the
// server drains, and the reference manifests giving the process more time than
// its own budget. Design: docs/design/graceful-shutdown.md.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// workStreams are the streaming handlers that deliberately ignore the drain
// latch because the connection *is* the work rather than a view of it. Killing
// one at rung 4 would destroy what draining exists to protect.
//
// Adding a file here is a decision, not a formality: say why.
var workStreams = map[string]string{
	// A worker's managed inference call. llmremote never retries — replaying a
	// call that already emitted deltas would duplicate output — so cutting one
	// fails the run. In k8s_job mode the worker outlives this server anyway.
	"internal/server/handlers/llmhttp/llmhttp.go": "a worker's inference call, drained as an ordinary request",
	// A Tier 1 turn streaming its own answer back. No other instance can take
	// over a turn in progress, so it finishes or it is lost.
	"internal/server/handlers/work/conversations.go": "a conversation turn producing its answer",
}

// TestEveryStreamingHandlerIsClassified fails when a handler starts writing SSE
// without deciding what it does on shutdown. The failure mode it prevents is
// silent: an unclassified watcher stream holds the drain open for the whole
// budget and is then severed anyway.
func TestEveryStreamingHandlerIsClassified(t *testing.T) {
	root := repoRoot(t)
	serverDir := filepath.Join(root, "internal", "server")

	err := filepath.WalkDir(serverDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		source := string(body)
		if !strings.Contains(source, "text/event-stream") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if _, ok := workStreams[rel]; ok {
			return nil
		}
		if !strings.Contains(source, "Drain") {
			t.Errorf("%s writes text/event-stream but neither observes the drain latch nor is listed in workStreams", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", serverDir, err)
	}
}

// TestManifestsOutliveTheShutdownBudget keeps the one misconfiguration that
// silently disables the whole ladder out of the reference deployments: a kill
// deadline shorter than the orderly stop it is supposed to allow.
func TestManifestsOutliveTheShutdownBudget(t *testing.T) {
	root := repoRoot(t)
	for _, manifest := range []string{
		filepath.Join("deployment", "buildmax-deploy.yaml"),
		filepath.Join("deployment", "production", "buildmax.yaml"),
	} {
		path := filepath.Join(root, manifest)
		serverYAML := configMapServerYAML(t, path)
		var cfg struct {
			ShutdownGrace string `yaml:"shutdown_grace"`
		}
		if err := yaml.Unmarshal([]byte(serverYAML), &cfg); err != nil {
			t.Fatalf("%s: parse server.yaml: %v", manifest, err)
		}
		if cfg.ShutdownGrace == "" {
			t.Errorf("%s: server.yaml sets no shutdown_grace", manifest)
			continue
		}
		grace, err := time.ParseDuration(cfg.ShutdownGrace)
		if err != nil {
			t.Fatalf("%s: shutdown_grace %q: %v", manifest, cfg.ShutdownGrace, err)
		}

		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", manifest, err)
		}
		period := serverTerminationGracePeriod(t, manifest, string(body))
		if float64(period) <= grace.Seconds() {
			t.Errorf("%s: terminationGracePeriodSeconds = %d, which does not outlast shutdown_grace %v", manifest, period, grace)
		}
	}
}

// serverTerminationGracePeriod reads the field off the buildmax-server
// Deployment, which is the pod the shutdown ladder runs in.
func serverTerminationGracePeriod(t *testing.T, name, body string) int {
	t.Helper()
	dec := yaml.NewDecoder(strings.NewReader(body))
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Spec struct {
				Template struct {
					Spec struct {
						TerminationGracePeriodSeconds *int `yaml:"terminationGracePeriodSeconds"`
					} `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"spec"`
		}
		if err := dec.Decode(&doc); err != nil {
			break
		}
		if doc.Kind != "Deployment" || doc.Metadata.Name != "buildmax-server" {
			continue
		}
		if doc.Spec.Template.Spec.TerminationGracePeriodSeconds == nil {
			t.Fatalf("%s: buildmax-server Deployment sets no terminationGracePeriodSeconds", name)
		}
		return *doc.Spec.Template.Spec.TerminationGracePeriodSeconds
	}
	t.Fatalf("%s: no buildmax-server Deployment", name)
	return 0
}
