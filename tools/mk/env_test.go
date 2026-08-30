package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLocalEnv puts a file where loadDotEnv looks for it, so a test cannot
// pass by writing to the path the loader used to read.
func writeLocalEnv(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(localEnvPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	content := "" +
		"# a comment\n" +
		"\n" +
		"PLAIN=value\n" +
		"export EXPORTED=exported-value\n" +
		"QUOTED=\"quoted value\"\n" +
		"SINGLE='single value'\n" +
		"SPACED = spaced \n" +
		"EMPTY=\n" +
		"URL=https://example.com/path?a=b\n" +
		"OVERRIDE=from-file\r\n"
	writeLocalEnv(t, dir, content)
	t.Setenv("OVERRIDE", "from-environment")

	if err := loadDotEnv(dir); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}

	want := map[string]string{
		"PLAIN":    "value",
		"EXPORTED": "exported-value",
		"QUOTED":   "quoted value",
		"SINGLE":   "single value",
		"SPACED":   "spaced",
		"EMPTY":    "",
		// Only the first "=" separates key from value.
		"URL": "https://example.com/path?a=b",
		// Matches `set -a; source .env`: the file wins over the inherited value.
		"OVERRIDE": "from-file",
	}
	for key, value := range want {
		t.Setenv(key, value) // registers cleanup for keys loadDotEnv set directly
		if got := os.Getenv(key); got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
}

func TestLoadDotEnvMissingFileIsNotAnError(t *testing.T) {
	if err := loadDotEnv(t.TempDir()); err != nil {
		t.Fatalf("loadDotEnv on a checkout without %s: %v", localEnvPath, err)
	}
}

// A .env left at the repository root is not read. `setup local` moves it, and
// doctor reports one that is still there; silently loading both paths would
// leave two files claiming to configure the same run.
func TestLoadDotEnvIgnoresARootDotEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("LEGACY_ONLY=set\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadDotEnv(dir); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv("LEGACY_ONLY"); got != "" {
		t.Errorf("LEGACY_ONLY = %q, want it unset", got)
	}
}

func TestLoadDotEnvRejectsMalformedLine(t *testing.T) {
	dir := t.TempDir()
	writeLocalEnv(t, dir, "NOT_A_PAIR\n")
	if err := loadDotEnv(dir); err == nil {
		t.Fatal("expected an error for a line without '='")
	}
}

func TestParseVersion(t *testing.T) {
	cases := []struct {
		tag                 string
		major, minor, patch int
		wantErr             bool
	}{
		{tag: "v0.1.0", major: 0, minor: 1, patch: 0},
		{tag: "v1.2.3", major: 1, minor: 2, patch: 3},
		{tag: "v0.1.0-alpha", major: 0, minor: 1, patch: 0},
		{tag: "v0.0.0", major: 0, minor: 0, patch: 0},
		{tag: "v2", major: 2, minor: 0, patch: 0},
		{tag: "vX.Y.Z", wantErr: true},
	}
	for _, tc := range cases {
		major, minor, patch, err := parseVersion(tc.tag)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseVersion(%q): expected an error", tc.tag)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseVersion(%q): %v", tc.tag, err)
			continue
		}
		if major != tc.major || minor != tc.minor || patch != tc.patch {
			t.Errorf("parseVersion(%q) = %d.%d.%d, want %d.%d.%d",
				tc.tag, major, minor, patch, tc.major, tc.minor, tc.patch)
		}
	}
}

func TestNextAlphaVersion(t *testing.T) {
	cases := []struct {
		current string
		want    string
		wantErr bool
	}{
		{current: "v0.1.0-alpha", want: "v0.1.0-alpha.1"},
		{current: "v0.2.0-alpha.4", want: "v0.2.0-alpha.5"},
		{current: "v10.20.30-alpha.99", want: "v10.20.30-alpha.100"},
		{current: "v0.2.0", wantErr: true},
		{current: "v0.2.0-beta.1", wantErr: true},
		{current: "v0.02.0-alpha.1", wantErr: true},
		{current: "not-a-version", wantErr: true},
	}
	for _, tc := range cases {
		got, err := nextAlphaVersion(tc.current)
		if tc.wantErr {
			if err == nil {
				t.Errorf("nextAlphaVersion(%q): expected an error", tc.current)
			}
			continue
		}
		if err != nil {
			t.Errorf("nextAlphaVersion(%q): %v", tc.current, err)
			continue
		}
		if got != tc.want {
			t.Errorf("nextAlphaVersion(%q) = %q, want %q", tc.current, got, tc.want)
		}
	}
}

// mk imports nothing from internal, so the qualification variables it names are
// a copy. A copy that drifts sends an operator to set a variable nothing reads,
// which is worse than not offering the command at all.
func TestCacheQualifyEnvNamesMatchTheSpec(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "internal", "config", "env_spec.go"))
	if err != nil {
		t.Fatalf("read env_spec.go: %v", err)
	}
	for _, name := range []string{
		envCacheQualifyProvider, envCacheQualifyModel, envCacheQualifyAPIKey, envCacheQualifyBaseURL,
		envCredentialStore,
	} {
		if !strings.Contains(string(spec), `"`+name+`"`) {
			t.Errorf("%s is not declared in internal/config/env_spec.go", name)
		}
	}
}
