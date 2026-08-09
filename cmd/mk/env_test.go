package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
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
		t.Fatalf("loadDotEnv on a directory without .env: %v", err)
	}
}

func TestLoadDotEnvRejectsMalformedLine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("NOT_A_PAIR\n"), 0o600); err != nil {
		t.Fatal(err)
	}
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
