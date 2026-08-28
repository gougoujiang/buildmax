package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMaxParallelTools(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   int
		want int
	}{
		{"unset takes the default", 0, DefaultMaxParallelTools},
		{"one disables parallelism", 1, 1},
		{"in range passes through", 8, 8},
		{"negative clamps up", -5, MinMaxParallelTools},
		{"too large clamps down", 999, MaxMaxParallelTools},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveMaxParallelTools(AgentConfig{MaxParallelTools: tc.in}); got != tc.want {
				t.Errorf("ResolveMaxParallelTools(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveMaxIterations(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings int
		override int
		want     int
	}{
		{"both unset takes the default", 0, 0, DefaultMaxIterations},
		{"settings alone applies", 400, 0, 400},
		{"override outranks settings", 400, 1200, 1200},
		{"override alone applies", 0, 1200, 1200},
		{"negative clamps up", 0, -5, MinMaxIterations},
		{"too large clamps down", 0, 99999, MaxMaxIterations},
		{"too large in settings clamps down", 99999, 0, MaxMaxIterations},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveMaxIterations(AgentConfig{MaxIterations: tc.settings}, tc.override)
			if got != tc.want {
				t.Errorf("ResolveMaxIterations(settings=%d, override=%d) = %d, want %d",
					tc.settings, tc.override, got, tc.want)
			}
		})
	}
}

// TestLoadSettings_ReadsAgentBlock guards the mapstructure tag. A wrong tag
// does not fail — it silently yields zero, which resolves to the default, so
// the setting would appear to work while doing nothing.
func TestLoadSettings_ReadsAgentBlock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, dir)
	if err := os.WriteFile(filepath.Join(dir, "settings.yaml"),
		[]byte("agent:\n  max_parallel_tools: 7\n  max_iterations: 600\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got.Agent.MaxParallelTools != 7 {
		t.Errorf("Agent.MaxParallelTools = %d, want 7 from settings.yaml", got.Agent.MaxParallelTools)
	}
	if got.Agent.MaxIterations != 600 {
		t.Errorf("Agent.MaxIterations = %d, want 600 from settings.yaml", got.Agent.MaxIterations)
	}
}
