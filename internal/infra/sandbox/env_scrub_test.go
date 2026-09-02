package sandbox

import (
	"slices"
	"testing"
)

func TestIsSecretEnvName(t *testing.T) {
	cases := []struct {
		name   string
		secret bool
	}{
		{"BUILDMAX_API_KEY", true},
		{"buildmax_api_key", true}, // case-insensitive
		{"AWS_SECRET_ACCESS_KEY", true},
		{"AWS_SESSION_TOKEN", true},
		{"GITHUB_TOKEN", true},
		{"OPENAI_API_KEY", true},
		{"STRIPE_SECRET_KEY", true}, // suffix _KEY (also _SECRET earlier in name)
		{"FOO_PASSWORD", true},
		{"FOO_PWD", true},
		{"FOO_PASSWD", true},
		// Negatives — must NOT match.
		{"PATH", false},
		{"HOME", false},
		{"USER", false},
		{"BUILDMAX_HOME", false},
		{"LANG", false},
		{"GOPATH", false},
		{"GOCACHE", false},
		{"", false},
		// Single bare word — we deliberately require an underscore so
		// "KEY" or "TOKEN" as a literal env name (rare) does not
		// over-match.
		{"KEY", false},
		{"TOKEN", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSecretEnvName(tc.name); got != tc.secret {
				t.Errorf("isSecretEnvName(%q) = %v, want %v", tc.name, got, tc.secret)
			}
		})
	}
}

func TestScrubEnvList(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"HOME=/home/me",
		"BUILDMAX_API_KEY=sk-secret",
		"GITHUB_TOKEN=ghp_abc",
		"AWS_SECRET_ACCESS_KEY=abc",
		"GOPATH=/go",
		"WEIRD_NO_EQUALS",            // kept (not KEY=VALUE shape)
		"STRIPE_SECRET_KEY=sk_test_", // dropped (matches both _SECRET and _KEY)
		"MY_PASSWORD=hunter2",
	}
	got := ScrubEnvList(in, nil)
	want := []string{"PATH=/usr/bin", "HOME=/home/me", "GOPATH=/go", "WEIRD_NO_EQUALS"}
	if !slices.Equal(got, want) {
		t.Errorf("ScrubEnvList:\n got %v\nwant %v", got, want)
	}
}

// TestScrubEnvList_AllowsDeclaredGrants proves a run's declared grant name
// passes even though it is secret-shaped, while BuildMax's own credential is
// never admitted, whatever the allow-list says.
func TestScrubEnvList_AllowsDeclaredGrants(t *testing.T) {
	in := []string{
		"GITHUB_TOKEN=ghp_abc",      // declared grant -> kept
		"AWS_SECRET_ACCESS_KEY=abc", // not declared -> dropped
		"BUILDMAX_RUN_TOKEN=rt",     // always denied even if declared
		"PATH=/usr/bin",
	}
	allowed := map[string]bool{"GITHUB_TOKEN": true, "BUILDMAX_RUN_TOKEN": true}
	got := ScrubEnvList(in, allowed)
	want := []string{"GITHUB_TOKEN=ghp_abc", "PATH=/usr/bin"}
	if !slices.Equal(got, want) {
		t.Errorf("ScrubEnvList with allow-list:\n got %v\nwant %v", got, want)
	}
}

// TestManagerAllowEnvNames_DropsAlwaysDeny proves the allow-list can never
// re-admit BuildMax's own credentials, whatever names are passed.
func TestManagerAllowEnvNames_DropsAlwaysDeny(t *testing.T) {
	m := &Manager{}
	m.AllowEnvNames([]string{"GH_TOKEN", "BUILDMAX_RUN_TOKEN", ""})
	if !m.allowedEnvNames["GH_TOKEN"] {
		t.Error("GH_TOKEN should be allow-listed")
	}
	if m.allowedEnvNames["BUILDMAX_RUN_TOKEN"] {
		t.Error("BUILDMAX_RUN_TOKEN must never be allow-listed")
	}
	if _, ok := m.allowedEnvNames[""]; ok {
		t.Error("empty name must not be stored")
	}
}
