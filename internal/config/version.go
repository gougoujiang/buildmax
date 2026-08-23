package config

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// Version and Commit identify the build. The release pipeline and ./make build
// inject both at link time so the git tag stays the single source of truth for
// what a binary calls itself:
//
//	-ldflags "-X github.com/gougoujiang/buildmax/internal/config.Version=0.1.0 \
//	          -X github.com/gougoujiang/buildmax/internal/config.Commit=abc1234"
//
// When the linker sets neither — `go install module@version`, `go build`,
// `go run`, `go test` — init falls back to the Go build info, so a binary
// installed with `go install` reports the released version it was built from
// instead of a bare "dev". See resolveBuildInfo.
var (
	// Version is the BuildMax application version, without a leading "v".
	Version = "dev"

	// Commit is the short git commit SHA the binary was built from.
	Commit = "dev"
)

// devPlaceholder is the value both variables hold until something better is
// known. Only a placeholder is overwritten, so an explicit -ldflags value
// always wins over the build info.
const devPlaceholder = "dev"

// UserAgent identifies BuildMax and the calling surface in outbound requests.
// viaGateway says BuildMax forwarded the call through its managed gateway.
func UserAgent(surface string, viaGateway bool) string {
	value := "buildmax/" + Version
	details := make([]string, 0, 2)
	if surface != "" {
		details = append(details, surface)
	}
	if viaGateway {
		details = append(details, "gateway")
	}
	if len(details) == 0 {
		return value
	}
	return value + " (" + strings.Join(details, "; ") + ")"
}

// pseudoVersion matches the tail of a Go pseudo-version: a build timestamp and
// a commit prefix, which the toolchain synthesizes for a commit that carries no
// release tag. Every pseudo-version form ends this way, whether the base is a
// bare 0.0.0 or an earlier tag.
var pseudoVersion = regexp.MustCompile(`[.-]\d{14}-[0-9a-f]{12}$`)

func init() {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	Version, Commit = resolveBuildInfo(Version, Commit, bi)
}

// resolveBuildInfo fills in whichever of version and commit is still the "dev"
// placeholder from bi, and returns both.
//
// The two inputs come from different places and neither is always present:
//
//   - `go install github.com/gougoujiang/buildmax/cmd/buildmax@latest` records
//     the module version in bi.Main.Version but carries no VCS stamp, so the
//     version is recovered and the commit is not.
//   - `go build` inside a checkout records vcs.revision and vcs.modified, and
//     bi.Main.Version is a pseudo-version derived from that same commit.
//
// Either half is better than reporting "dev" for both, which is what every
// `go install` user saw before this fallback existed and what made version
// numbers in bug reports useless. A pseudo-version is only used when no commit
// was recovered, since where both exist it is a longer spelling of the commit.
func resolveBuildInfo(version, commit string, bi *debug.BuildInfo) (string, string) {
	if bi == nil {
		return version, commit
	}
	revision, modified := vcsStamp(bi)

	if commit == devPlaceholder && len(revision) >= 7 {
		commit = revision[:7]
		if modified {
			// An uncommitted tree is not the commit it claims to be, and a bug
			// report from one should say so.
			commit += "-dirty"
		}
	}

	if version == devPlaceholder {
		// "+dirty" repeats what the commit already records.
		v := strings.TrimSuffix(strings.TrimPrefix(bi.Main.Version, "v"), "+dirty")
		switch {
		case v == "" || v == "(devel)" || v == devPlaceholder:
		case pseudoVersion.MatchString(v) && commit != devPlaceholder:
		default:
			version = v
		}
	}
	return version, commit
}

// vcsStamp returns the revision and dirty flag the toolchain recorded, or the
// zero values when the build carried no VCS information.
func vcsStamp(bi *debug.BuildInfo) (revision string, modified bool) {
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return revision, modified
}

// VersionString returns the human-readable version string, e.g. "0.1.0 (abc1234)".
// The commit is omitted when it is unknown, which is the normal case for a
// `go install` build.
func VersionString() string {
	if Commit == "" || Commit == devPlaceholder {
		return Version
	}
	return Version + " (" + Commit + ")"
}
