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

// TestLoadSettings_ReadsAgentBlock guards the mapstructure tag. A wrong tag
// does not fail — it silently yields zero, which resolves to the default, so
// the setting would appear to work while doing nothing.
func TestLoadSettings_ReadsAgentBlock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, dir)
	if err := os.WriteFile(filepath.Join(dir, "settings.yaml"),
		[]byte("agent:\n  max_parallel_tools: 7\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got.Agent.MaxParallelTools != 7 {
		t.Errorf("Agent.MaxParallelTools = %d, want 7 from settings.yaml", got.Agent.MaxParallelTools)
	}
}
