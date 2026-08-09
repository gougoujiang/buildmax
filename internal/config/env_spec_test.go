package config

import "testing"

func TestEnvVarsHaveUniqueNames(t *testing.T) {
	seen := map[string]struct{}{}
	for _, envVar := range EnvVars {
		if _, ok := seen[envVar.Name]; ok {
			t.Fatalf("EnvVars contains duplicate env name %q", envVar.Name)
		}
		seen[envVar.Name] = struct{}{}
	}
}

func TestEnvVarsIncludeRequiredKeys(t *testing.T) {
	required := []string{EnvKeyBuildmaxHome, EnvKeyBuildmaxJWTSecret}
	names := map[string]struct{}{}
	for _, e := range EnvVars {
		names[e.Name] = struct{}{}
	}
	for _, k := range required {
		if _, ok := names[k]; !ok {
			t.Errorf("EnvVars missing required key %q", k)
		}
	}
}
