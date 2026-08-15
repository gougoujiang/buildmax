package config

import (
	"runtime/debug"
	"testing"
)

func buildInfo(mainVersion string, settings ...debug.BuildSetting) *debug.BuildInfo {
	return &debug.BuildInfo{
		Main:     debug.Module{Version: mainVersion},
		Settings: settings,
	}
}

func TestResolveBuildInfo(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		commit      string
		bi          *debug.BuildInfo
		wantVersion string
		wantCommit  string
	}{
		{
			name:        "ldflags win over build info",
			version:     "0.1.0",
			commit:      "abc1234",
			bi:          buildInfo("v9.9.9", debug.BuildSetting{Key: "vcs.revision", Value: "ffffffffffffffff"}),
			wantVersion: "0.1.0",
			wantCommit:  "abc1234",
		},
		{
			name:        "go install a tagged version",
			version:     "dev",
			commit:      "dev",
			bi:          buildInfo("v0.1.0-alpha"),
			wantVersion: "0.1.0-alpha",
			wantCommit:  "dev",
		},
		{
			name:        "go install an untagged commit",
			version:     "dev",
			commit:      "dev",
			bi:          buildInfo("v0.0.0-20260809120000-abcdef123456"),
			wantVersion: "0.0.0-20260809120000-abcdef123456",
			wantCommit:  "dev",
		},
		{
			name:    "go build inside a clean checkout",
			version: "dev",
			commit:  "dev",
			bi: buildInfo("(devel)",
				debug.BuildSetting{Key: "vcs.revision", Value: "1061146abcdef0000000"},
				debug.BuildSetting{Key: "vcs.modified", Value: "false"},
			),
			wantVersion: "dev",
			wantCommit:  "1061146",
		},
		{
			name:    "go build inside a dirty checkout",
			version: "dev",
			commit:  "dev",
			bi: buildInfo("(devel)",
				debug.BuildSetting{Key: "vcs.revision", Value: "1061146abcdef0000000"},
				debug.BuildSetting{Key: "vcs.modified", Value: "true"},
			),
			wantVersion: "dev",
			wantCommit:  "1061146-dirty",
		},
		{
			// Go 1.24 and later synthesize a pseudo-version for a local build.
			// It spells out the same commit the VCS stamp already carries, so
			// the shorter form wins.
			name:    "pseudo-version alongside a vcs stamp",
			version: "dev",
			commit:  "dev",
			bi: buildInfo("v0.1.0-alpha.0.20260815055010-07f9d1a7b8ba+dirty",
				debug.BuildSetting{Key: "vcs.revision", Value: "07f9d1a7b8ba0000000"},
				debug.BuildSetting{Key: "vcs.modified", Value: "true"},
			),
			wantVersion: "dev",
			wantCommit:  "07f9d1a-dirty",
		},
		{
			name:        "no version and no vcs stamp",
			version:     "dev",
			commit:      "dev",
			bi:          buildInfo(""),
			wantVersion: "dev",
			wantCommit:  "dev",
		},
		{
			name:        "revision too short to shorten",
			version:     "dev",
			commit:      "dev",
			bi:          buildInfo("(devel)", debug.BuildSetting{Key: "vcs.revision", Value: "abc"}),
			wantVersion: "dev",
			wantCommit:  "dev",
		},
		{
			name:        "nil build info",
			version:     "dev",
			commit:      "dev",
			bi:          nil,
			wantVersion: "dev",
			wantCommit:  "dev",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotVersion, gotCommit := resolveBuildInfo(tc.version, tc.commit, tc.bi)
			if gotVersion != tc.wantVersion || gotCommit != tc.wantCommit {
				t.Errorf("resolveBuildInfo() = (%q, %q), want (%q, %q)",
					gotVersion, gotCommit, tc.wantVersion, tc.wantCommit)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	original := []string{Version, Commit}
	t.Cleanup(func() { Version, Commit = original[0], original[1] })

	tests := []struct {
		version string
		commit  string
		want    string
	}{
		{"0.1.0", "abc1234", "0.1.0 (abc1234)"},
		{"0.1.0-alpha", devPlaceholder, "0.1.0-alpha"}, // go install: no VCS stamp
		{devPlaceholder, "abc1234", "dev (abc1234)"},
		{devPlaceholder, devPlaceholder, "dev"},
		{"0.1.0", "", "0.1.0"},
	}
	for _, tc := range tests {
		Version, Commit = tc.version, tc.commit
		if got := VersionString(); got != tc.want {
			t.Errorf("VersionString() with (%q, %q) = %q, want %q", tc.version, tc.commit, got, tc.want)
		}
	}
}

// The version reported by a test binary is whatever the toolchain stamped, but
// it must never be empty — VersionString goes into --version output and into
// the server startup log.
func TestVersionStringIsNeverEmpty(t *testing.T) {
	if VersionString() == "" {
		t.Error("VersionString() is empty")
	}
}
