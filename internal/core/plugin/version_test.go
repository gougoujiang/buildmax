package plugin

import "testing"

func TestParseVersionValid(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"1.2.0", "1.2.0"},
		{"0.0.0", "0.0.0"},
		{"1.3.0-rc.1", "1.3.0-rc.1"},
		{"0.1.0-alpha", "0.1.0-alpha"},
		{"1.2.3+build.5", "1.2.3"},
		{" 1.2.3 ", "1.2.3"},
	}
	for _, tc := range tests {
		v, err := ParseVersion(tc.in)
		if err != nil {
			t.Errorf("ParseVersion(%q): %v", tc.in, err)
			continue
		}
		if got := v.String(); got != tc.want {
			t.Errorf("ParseVersion(%q).String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseVersionInvalid(t *testing.T) {
	for _, in := range []string{
		"", "1.2", "1.2.3.4", "v1.2.3", "1.2.x", "01.2.3", "1.02.3",
		">=1.2.3", "1.2.3-", "1.2.3-rc..1", "1.2.3-rc/1", "latest",
	} {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) = nil error, want a rejection", in)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.1.0", "1.0.9", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-rc.1", "1.0.0-rc.2", -1},
		{"1.0.0-rc.2", "1.0.0-rc.10", -1},
		{"1.0.0-rc", "1.0.0-rc.1", -1},
		{"1.0.0-1", "1.0.0-alpha", -1},
		{"1.0.0+a", "1.0.0+b", 0},
	}
	for _, tc := range tests {
		a, err := ParseVersion(tc.a)
		if err != nil {
			t.Fatalf("ParseVersion(%q): %v", tc.a, err)
		}
		b, err := ParseVersion(tc.b)
		if err != nil {
			t.Fatalf("ParseVersion(%q): %v", tc.b, err)
		}
		if got := a.Compare(b); got != tc.want {
			t.Errorf("%q.Compare(%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestVersionIsRelease(t *testing.T) {
	release, _ := ParseVersion("1.2.3")
	pre, _ := ParseVersion("1.2.3-rc.1")
	if !release.IsRelease() {
		t.Error("1.2.3 should be a release")
	}
	if pre.IsRelease() {
		t.Error("1.2.3-rc.1 should not be a release")
	}
}

// The inputs are the version strings internal/config can actually produce:
// an ldflags tag, a `git describe` between tags, a Go pseudo-version, a
// prerelease tag, and the "dev" placeholder.
func TestParseClientVersion(t *testing.T) {
	tests := []struct {
		in        string
		wantKnown bool
		want      string
	}{
		{"0.1.0", true, "0.1.0"},
		{"0.1.0-alpha", true, "0.1.0-alpha"},
		{"0.1.0-3-gabc1234", true, "0.1.0"},
		{"1.2.0-12-g0011223", true, "1.2.0"},
		{"dev", false, ""},
		{"", false, ""},
		{"0.0.0-20260809120000-abcdef123456", false, ""},
		{"nonsense", false, ""},
	}
	for _, tc := range tests {
		got := ParseClientVersion(tc.in)
		if got.Known != tc.wantKnown {
			t.Errorf("ParseClientVersion(%q).Known = %v, want %v", tc.in, got.Known, tc.wantKnown)
			continue
		}
		if got.Known && got.Version.String() != tc.want {
			t.Errorf("ParseClientVersion(%q) = %q, want %q", tc.in, got.Version, tc.want)
		}
	}
}

func TestClientVersionSatisfies(t *testing.T) {
	bound, err := ParseVersion("0.9.0")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		client string
		want   bool
	}{
		{"0.9.0", true},
		{"0.9.1", true},
		{"1.0.0", true},
		{"0.8.9", false},
		// Three commits after v0.9.0 contains 0.9.0, even though semver would
		// read the suffix as a prerelease and sort it below.
		{"0.9.0-3-gabc1234", true},
		// A prerelease of the bound itself does not contain it.
		{"0.9.0-rc.1", false},
		// An unplaceable build is never blocked.
		{"dev", true},
		{"0.0.0-20260809120000-abcdef123456", true},
	}
	for _, tc := range tests {
		if got := ParseClientVersion(tc.client).Satisfies(bound); got != tc.want {
			t.Errorf("client %q satisfies 0.9.0 = %v, want %v", tc.client, got, tc.want)
		}
	}
}
