package plugin

import (
	"errors"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

func release(version string, opts ...func(*model.PluginRelease)) model.PluginRelease {
	r := model.PluginRelease{PluginName: "code-review", Version: version}
	for _, o := range opts {
		o(&r)
	}
	return r
}

func yanked(reason string) func(*model.PluginRelease) {
	return func(r *model.PluginRelease) { r.YankedAt, r.YankedReason = 1, reason }
}

func needs(v string) func(*model.PluginRelease) {
	return func(r *model.PluginRelease) { r.MinBuildmaxVersion = v }
}

func TestSelectReleaseDefaultTakesTheNewestStable(t *testing.T) {
	got, err := SelectRelease([]model.PluginRelease{
		release("1.0.0"), release("1.10.0"), release("1.9.0"), release("2.0.0-rc.1"),
	}, SelectOptions{ClientVersion: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	// 1.10.0 beats 1.9.0, which lexical ordering would get backwards, and a
	// prerelease is not the default even when it is newest.
	if got.Version != "1.10.0" {
		t.Errorf("selected %s, want 1.10.0", got.Version)
	}
}

func TestSelectReleaseSkipsWhatTheDefaultShouldNotTake(t *testing.T) {
	releases := []model.PluginRelease{
		release("1.0.0"),
		release("1.1.0", yanked("broken hook")),
		release("1.2.0", needs("9.0.0")),
		release("1.3.0-rc.1"),
	}
	got, err := SelectRelease(releases, SelectOptions{ClientVersion: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.0.0" {
		t.Errorf("selected %s, want 1.0.0", got.Version)
	}

	// Every one of those exclusions is recoverable by naming the version.
	for _, tc := range []struct {
		version string
		opts    SelectOptions
	}{
		{"1.1.0", SelectOptions{ClientVersion: "1.0.0", AllowYanked: true}},
		{"1.2.0", SelectOptions{ClientVersion: "9.0.0"}},
		{"1.3.0-rc.1", SelectOptions{ClientVersion: "1.0.0"}},
	} {
		opts := tc.opts
		opts.Version = tc.version
		got, err := SelectRelease(releases, opts)
		if err != nil {
			t.Errorf("naming %s: %v", tc.version, err)
			continue
		}
		if got.Version != tc.version {
			t.Errorf("selected %s, want %s", got.Version, tc.version)
		}
	}
}

func TestSelectReleaseRefusals(t *testing.T) {
	releases := []model.PluginRelease{
		release("1.0.0", yanked("broken hook")),
		release("2.0.0", needs("9.0.0")),
	}
	tests := []struct {
		name string
		opts SelectOptions
		want error
	}{
		{"nothing fits", SelectOptions{ClientVersion: "1.0.0"}, ErrNoRelease},
		{"no such version", SelectOptions{Version: "3.0.0", ClientVersion: "1.0.0"}, ErrNoRelease},
		{"withdrawn", SelectOptions{Version: "1.0.0", ClientVersion: "1.0.0"}, ErrReleaseYanked},
		{"build too old", SelectOptions{Version: "2.0.0", ClientVersion: "1.0.0"}, ErrClientTooOld},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SelectRelease(releases, tc.opts)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}

	// A refusal names what was wrong, so a recovery knows what to acknowledge.
	_, err := SelectRelease(releases, SelectOptions{Version: "1.0.0", ClientVersion: "1.0.0"})
	if err == nil || !strings.Contains(err.Error(), "broken hook") {
		t.Errorf("err = %v, want the withdrawal reason", err)
	}
	_, err = SelectRelease(releases, SelectOptions{Version: "2.0.0", ClientVersion: "1.0.0"})
	if err == nil || !strings.Contains(err.Error(), "9.0.0") {
		t.Errorf("err = %v, want the bound", err)
	}
}

// Refusing to install on a version nobody can compare would break every
// contributor's checkout.
func TestSelectReleaseUnplaceableClientSatisfiesEveryBound(t *testing.T) {
	releases := []model.PluginRelease{release("1.0.0", needs("9.9.9"))}
	for _, client := range []string{"", "dev", "0.0.0-20260809120000-abcdef123456"} {
		got, err := SelectRelease(releases, SelectOptions{ClientVersion: client})
		if err != nil {
			t.Errorf("client %q: %v", client, err)
			continue
		}
		if got.Version != "1.0.0" {
			t.Errorf("client %q selected %s", client, got.Version)
		}
	}
	// A build three commits after the tag contains it, which semver alone
	// would read the other way.
	got, err := SelectRelease([]model.PluginRelease{release("1.0.0", needs("0.9.0"))},
		SelectOptions{ClientVersion: "0.9.0-3-gabc1234"})
	if err != nil || got == nil {
		t.Errorf("a build after the tag should satisfy it: %v", err)
	}
}

// Publishing rejects an unparseable version, so one in the catalog came from a
// build that did not. Skipping it beats ranking it by guess.
func TestSelectReleaseSkipsAnUnorderableVersion(t *testing.T) {
	got, err := SelectRelease([]model.PluginRelease{
		release("not-a-version"), release("1.0.0"),
	}, SelectOptions{ClientVersion: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.0.0" {
		t.Errorf("selected %s, want 1.0.0", got.Version)
	}
	// Named exactly it is still reachable, because the bytes are still there.
	if _, err := SelectRelease([]model.PluginRelease{release("not-a-version")},
		SelectOptions{Version: "not-a-version"}); err != nil {
		t.Errorf("naming it exactly: %v", err)
	}
}

func TestSelectReleaseEmptyCatalog(t *testing.T) {
	if _, err := SelectRelease(nil, SelectOptions{}); !errors.Is(err, ErrNoRelease) {
		t.Errorf("err = %v, want ErrNoRelease", err)
	}
}
