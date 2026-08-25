package config

import "testing"

func TestEnvVarsHaveUniqueNames(t *testing.T) {
	seen := map[string]struct{}{}
	for _, envVar := range EnvVars() {
		if _, ok := seen[envVar.Name]; ok {
			t.Fatalf("EnvVars contains duplicate env name %q", envVar.Name)
		}
		seen[envVar.Name] = struct{}{}
	}
}

func TestEnvVarsIncludeRequiredKeys(t *testing.T) {
	required := []string{EnvKeyBuildmaxHome, EnvKeyBuildmaxJWTSecret}
	names := map[string]struct{}{}
	for _, e := range EnvVars() {
		names[e.Name] = struct{}{}
	}
	for _, k := range required {
		if _, ok := names[k]; !ok {
			t.Errorf("EnvVars missing required key %q", k)
		}
	}
}

func TestEnvVarsReturnsCopy(t *testing.T) {
	vars := EnvVars()
	vars[0].Name = "BROKEN"
	if got := EnvVars()[0].Name; got != EnvKeyBuildmaxHome {
		t.Fatalf("environment specification mutated through caller slice: %q", got)
	}
}
