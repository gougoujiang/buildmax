package plugin

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a semantic version. Build metadata is accepted and discarded: it
// never affects ordering, and keeping it would invite comparisons that do.
type Version struct {
	Major int
	Minor int
	Patch int
	// Pre holds prerelease identifiers, empty for a release version.
	Pre []string
}

var numericIdent = regexp.MustCompile(`^(0|[1-9]\d*)$`)

// ParseVersion parses a plugin or bound version. A leading "v" is rejected
// rather than trimmed, so one spelling reaches the catalog.
func ParseVersion(s string) (Version, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return Version{}, fmt.Errorf("empty version")
	}
	if strings.HasPrefix(raw, "v") {
		return Version{}, fmt.Errorf("version %q must not start with %q", raw, "v")
	}
	if i := strings.IndexByte(raw, '+'); i >= 0 {
		raw = raw[:i]
	}
	core, pre, hasPre := strings.Cut(raw, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("version %q is not major.minor.patch", s)
	}
	var v Version
	for i, p := range parts {
		if !numericIdent.MatchString(p) {
			return Version{}, fmt.Errorf("version %q: %q is not a number without leading zeros", s, p)
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, fmt.Errorf("version %q: %w", s, err)
		}
		switch i {
		case 0:
			v.Major = n
		case 1:
			v.Minor = n
		case 2:
			v.Patch = n
		}
	}
	if hasPre {
		if pre == "" {
			return Version{}, fmt.Errorf("version %q has an empty prerelease", s)
		}
		for _, id := range strings.Split(pre, ".") {
			if id == "" {
				return Version{}, fmt.Errorf("version %q has an empty prerelease identifier", s)
			}
			if !isPrereleaseIdent(id) {
				return Version{}, fmt.Errorf("version %q: prerelease %q is not alphanumeric or hyphen", s, id)
			}
			v.Pre = append(v.Pre, id)
		}
	}
	return v, nil
}

func isPrereleaseIdent(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-':
		default:
			return false
		}
	}
	return true
}

// IsRelease reports whether this version is not a prerelease. Default install
// selection skips everything else.
func (v Version) IsRelease() bool { return len(v.Pre) == 0 }

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}
	return s
}

// Compare orders two versions by semantic version precedence: -1, 0, or 1.
func (v Version) Compare(o Version) int {
	if c := cmpInt(v.Major, o.Major); c != 0 {
		return c
	}
	if c := cmpInt(v.Minor, o.Minor); c != 0 {
		return c
	}
	if c := cmpInt(v.Patch, o.Patch); c != 0 {
		return c
	}
	return comparePre(v.Pre, o.Pre)
}

// comparePre implements the semver rule that a prerelease sorts below the
// release it precedes, and that between two prereleases the identifiers decide.
func comparePre(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := comparePreIdent(a[i], b[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(a), len(b))
}

func comparePreIdent(a, b string) int {
	aNum, bNum := numericIdent.MatchString(a), numericIdent.MatchString(b)
	switch {
	case aNum && bNum:
		x, _ := strconv.Atoi(a)
		y, _ := strconv.Atoi(b)
		return cmpInt(x, y)
	case aNum:
		return -1
	case bNum:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// These match the version strings a BuildMax build can call itself. They are
// duplicated from internal/config rather than imported: core may not depend on
// config, and a bound check that silently misreads a build's own version is
// worse than one copied regexp.
var (
	pseudoVersionTail = regexp.MustCompile(`[.-]\d{14}-[0-9a-f]{12}$`)
	describeTail      = regexp.MustCompile(`^(.+)-\d+-g[0-9a-f]{7,}$`)
)

// ClientVersion is what a running build reports about itself, which is not
// always a release version.
type ClientVersion struct {
	Version Version
	// Known is false for a build whose version cannot be placed on the release
	// line at all — "dev", a Go pseudo-version, anything unparseable. Such a
	// build satisfies every bound, because refusing to run on an unknown
	// version would break every contributor's checkout.
	Known bool
}

// ParseClientVersion classifies a build's own version string.
//
// A `git describe` version such as "0.1.0-3-gabc1234" is three commits *after*
// v0.1.0, but semver reads that suffix as a prerelease and would sort it below
// 0.1.0. It is therefore reduced to its base tag, which is the newest release
// the build is known to contain.
func ParseClientVersion(s string) ClientVersion {
	raw := strings.TrimSpace(s)
	if raw == "" || raw == "dev" || pseudoVersionTail.MatchString(raw) {
		return ClientVersion{}
	}
	if m := describeTail.FindStringSubmatch(raw); m != nil {
		raw = m[1]
	}
	v, err := ParseVersion(raw)
	if err != nil {
		return ClientVersion{}
	}
	return ClientVersion{Version: v, Known: true}
}

// Satisfies reports whether this build meets a plugin's min_buildmax_version.
func (c ClientVersion) Satisfies(min Version) bool {
	return !c.Known || c.Version.Compare(min) >= 0
}
