package plugin

import (
	"strings"
	"testing"
)

func parseOK(t *testing.T, src string) (Manifest, []Finding) {
	t.Helper()
	m, findings, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return m, findings
}

// Only name is required to load a plugin. A skill-only plugin that never
// reaches a catalog should not have to fill in anything else.
func TestParseMinimalManifest(t *testing.T) {
	m, findings := parseOK(t, "name: code-review\n")
	if HasErrors(findings) {
		t.Fatalf("unexpected errors: %v", findings)
	}
	if m.Name != "code-review" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Version != "" || m.Env != nil || m.Unknown != nil {
		t.Errorf("unset fields should stay zero: %+v", m)
	}
	if m.DisplayTitle() != "code-review" {
		t.Errorf("DisplayTitle = %q, want the name", m.DisplayTitle())
	}
}

func TestParseFullManifest(t *testing.T) {
	src := `
name: code-review
version: 1.2.0
description: Company code review skills and agents.

display_name: Code Review
homepage: https://code.example.com/agents/code-review
maintainer: Platform Team <platform@example.com>
license: Apache-2.0

min_buildmax_version: 0.9.0

env:
  GITHUB_TOKEN:
    description: Token the github MCP server authenticates with.
  REVIEW_WEBHOOK_URL:
    description: Where the post_tool_use hook posts review results.
    required: false
`
	m, findings := parseOK(t, src)
	if HasErrors(findings) {
		t.Fatalf("unexpected errors: %v", findings)
	}
	if m.Version != "1.2.0" || m.MinBuildmaxVersion != "0.9.0" {
		t.Errorf("version fields: %+v", m)
	}
	if m.DisplayTitle() != "Code Review" {
		t.Errorf("DisplayTitle = %q", m.DisplayTitle())
	}
	if m.Maintainer != "Platform Team <platform@example.com>" {
		t.Errorf("Maintainer = %q", m.Maintainer)
	}
	if len(m.Env) != 2 {
		t.Fatalf("got %d env entries, want 2", len(m.Env))
	}
	// File order is kept so diagnostics and status read like the file.
	if m.Env[0].Name != "GITHUB_TOKEN" || m.Env[1].Name != "REVIEW_WEBHOOK_URL" {
		t.Errorf("env order not preserved: %+v", m.Env)
	}
	if !m.Env[0].Required {
		t.Error("required should default to true")
	}
	if m.Env[1].Required {
		t.Error("required: false should be honoured")
	}
	if e, ok := m.EnvVarByName("GITHUB_TOKEN"); !ok || !strings.Contains(e.Description, "github MCP") {
		t.Errorf("EnvVarByName: %+v ok=%v", e, ok)
	}
	if _, ok := m.EnvVarByName("NOPE"); ok {
		t.Error("EnvVarByName found a variable that was not declared")
	}
}

// An unknown field must load and be reported: silence would make a misspelling
// invisible, and an error would make every older client reject a newer plugin.
func TestParseUnknownFieldWarns(t *testing.T) {
	m, findings := parseOK(t, "name: demo\ndescripton: typo\nfuture_field: 3\n")
	if HasErrors(findings) {
		t.Fatalf("unknown fields must not block loading: %v", findings)
	}
	if len(m.Unknown) != 2 || m.Unknown[0] != "descripton" || m.Unknown[1] != "future_field" {
		t.Fatalf("Unknown = %v, want both keys in file order", m.Unknown)
	}
	var warned int
	for _, f := range findings {
		if f.Severity == SeverityWarning {
			warned++
			if f.Line == 0 {
				t.Errorf("unknown field finding has no line: %+v", f)
			}
		}
	}
	if warned != 2 {
		t.Errorf("got %d warnings, want 2", warned)
	}
}

func TestParseNameRules(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"code-review", false},
		{"a", false},
		{"team1-code-review2", false},
		{"", true},
		{"Code-Review", true},
		{"code_review", true},
		{"code--review", true},
		{"-code", true},
		{"code-", true},
		{"../escape", true},
		{"code.review", true},
		{strings.Repeat("a", maxNameLen+1), true},
	}
	for _, tc := range tests {
		err := ValidateName(tc.name)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateName(%q) = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
}

func TestParseMissingNameIsAnError(t *testing.T) {
	for _, src := range []string{"", "description: no name here\n", "name: \"\"\n"} {
		_, findings, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		if !HasErrors(findings) {
			t.Errorf("Parse(%q) produced no error finding", src)
		}
	}
}

func TestParseRejectsBadVersions(t *testing.T) {
	tests := []struct {
		src   string
		field string
	}{
		{"name: demo\nversion: v1.2.0\n", "version"},
		{"name: demo\nversion: 1.2\n", "version"},
		{"name: demo\nmin_buildmax_version: \">=0.9.0\"\n", "min_buildmax_version"},
		{"name: demo\nmin_buildmax_version: latest\n", "min_buildmax_version"},
	}
	for _, tc := range tests {
		_, findings := parseOK(t, tc.src)
		if !HasErrors(findings) {
			t.Errorf("%q: no error finding", tc.src)
			continue
		}
		found := false
		for _, f := range Errors(findings) {
			if f.Field == tc.field {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: no error on field %q, got %v", tc.src, tc.field, findings)
		}
	}
}

// The one place forward compatibility is refused: an unrecognised key under an
// env entry could be a checked-in secret.
func TestParseEnvRejectsAValue(t *testing.T) {
	src := `
name: demo
env:
  GITHUB_TOKEN:
    description: A token.
    value: ghp_realsecret
`
	_, findings := parseOK(t, src)
	if !HasErrors(findings) {
		t.Fatal("a value under an env entry must be an error")
	}
	var msg string
	for _, f := range Errors(findings) {
		if strings.HasPrefix(f.Field, "env.GITHUB_TOKEN") {
			msg = f.Message
		}
	}
	if !strings.Contains(msg, "never carry a value") {
		t.Errorf("error message should say why: %q", msg)
	}
}

func TestParseEnvShapeErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"env is not a mapping", "name: demo\nenv: [A, B]\n"},
		{"entry is not a mapping", "name: demo\nenv:\n  TOKEN: a token\n"},
		{"invalid variable name", "name: demo\nenv:\n  \"not a var\":\n    description: x\n"},
		{"required is not a bool", "name: demo\nenv:\n  TOKEN:\n    required: sometimes\n"},
		{"duplicate variable", "name: demo\nenv:\n  TOKEN:\n    description: a\n  TOKEN:\n    description: b\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, findings := parseOK(t, tc.src)
			if !HasErrors(findings) {
				t.Errorf("no error finding for %q", tc.src)
			}
		})
	}
}

func TestParseDuplicateTopLevelKey(t *testing.T) {
	_, findings := parseOK(t, "name: one\nname: two\n")
	if !HasErrors(findings) {
		t.Fatal("a duplicate key must be an error")
	}
	if !strings.Contains(Errors(findings)[0].Message, "first defined on line 1") {
		t.Errorf("error should point at the first definition: %v", findings)
	}
}

func TestParseWrongFieldType(t *testing.T) {
	_, findings := parseOK(t, "name:\n  nested: true\n")
	if !HasErrors(findings) {
		t.Fatal("a mapping where a string belongs must be an error")
	}
}

// A document that is not a manifest at all is the only hard failure: there is
// no partial result to report findings against.
func TestParseNotAManifest(t *testing.T) {
	for _, src := range []string{"- a\n- b\n", "just a string\n", "name: [unclosed\n"} {
		if _, _, err := Parse([]byte(src)); err == nil {
			t.Errorf("Parse(%q) = nil error, want a hard failure", src)
		}
	}
}

func TestFindingString(t *testing.T) {
	f := Finding{Severity: SeverityError, Field: "version", Line: 3, Message: "bad"}
	if got, want := f.String(), "plugin.yaml:3: error: version: bad"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	plain := Finding{Severity: SeverityWarning, Message: "no location"}
	if got, want := plain.String(), "warning: no location"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// A version that YAML reads as a number is the most likely version mistake, so
// it must be reported as a bad version rather than as a bad type.
func TestParseNumericVersionReportsSemver(t *testing.T) {
	_, findings := parseOK(t, "name: demo\nversion: 1.2\n")
	errs := Errors(findings)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "major.minor.patch") {
		t.Errorf("message = %q, want the semver reason", errs[0].Message)
	}
}
