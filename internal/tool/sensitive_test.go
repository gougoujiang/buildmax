package tool

import (
	"github.com/gougoujiang/buildmax/internal/util"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

func TestIsSensitivePath(t *testing.T) {
	sensitive := []string{
		".env",
		".env.local",
		".env.production",
		".env.test",
		"/home/user/.env",
		"id_rsa",
		"id_ed25519",
		"id_ecdsa",
		"id_dsa",
		"/home/user/.ssh/id_rsa",
		"server.key",
		"cert.pem",
		"bundle.p12",
		"keystore.pfx",
		"client.der",
		".netrc",
		".htpasswd",
		"credentials",
		"/home/user/.aws/credentials",
		"/home/user/.aws/config",
		"/Users/foo/.ssh/id_ed25519",
		"secrets/prod.key",
	}
	for _, p := range sensitive {
		if !isSensitivePath(p) {
			t.Errorf("isSensitivePath(%q) = false, want true", p)
		}
	}

	safe := []string{
		"main.go",
		"README.md",
		".gitignore",
		"config.yaml",
		"settings.json",
		"docker-compose.yml",
		"Makefile",
		"package.json",
		"go.sum",
		"cert.go",       // contains "cert" but is a Go file, no .crt extension
		"public.key.js", // extension is .js, not .key
		"environment.ts",
	}
	for _, p := range safe {
		if isSensitivePath(p) {
			t.Errorf("isSensitivePath(%q) = true, want false", p)
		}
	}
}

// TestSensitiveArgCheckers verifies that ReadFile, WriteFile, EditFile, and Grep
// return Ask for sensitive paths and Allow for safe paths.
func TestSensitiveArgCheckers(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		args    map[string]any
		argKey  string
		wantAsk bool
	}{
		{"read .env", map[string]any{"file_path": ".env"}, "file_path", true},
		{"read main.go", map[string]any{"file_path": "main.go"}, "file_path", false},
		{"write id_rsa", map[string]any{"file_path": "id_rsa", "content": ""}, "file_path", true},
		{"write config.yaml", map[string]any{"file_path": "config.yaml", "content": ""}, "file_path", false},
		{"edit .env.local", map[string]any{"file_path": ".env.local", "old_string": "x", "new_string": "y"}, "file_path", true},
		{"edit README.md", map[string]any{"file_path": "README.md", "old_string": "x", "new_string": "y"}, "file_path", false},
		{"grep path=.env", map[string]any{"pattern": "API_KEY", "path": ".env"}, "path", true},
		{"grep path=main.go", map[string]any{"pattern": "func", "path": "main.go"}, "path", false},
	}

	ws := testWorkspace(t, dir)
	rf := NewReadFile(util.FixedRoot(ws))
	wf := NewWriteFile(util.FixedRoot(ws))
	ef := NewEditFile(util.FixedRoot(ws))
	gp := NewGrep(util.FixedRoot(ws))

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got llm.ToolAction
			switch tc.argKey {
			case "file_path":
				if _, isWrite := tc.args["content"]; isWrite {
					got = wf.CheckArgs(tc.args)
				} else if _, isEdit := tc.args["old_string"]; isEdit {
					got = ef.CheckArgs(tc.args)
				} else {
					got = rf.CheckArgs(tc.args)
				}
			case "path":
				got = gp.CheckArgs(tc.args)
			}
			if tc.wantAsk && got != llm.ToolActionAsk {
				t.Errorf("CheckArgs(%v) = %v, want Ask", tc.args, got)
			}
			if !tc.wantAsk && got != llm.ToolActionAllow {
				t.Errorf("CheckArgs(%v) = %v, want Allow", tc.args, got)
			}
		})
	}
}
