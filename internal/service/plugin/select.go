package plugin

import (
	"errors"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/model"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
)

// Reasons a release cannot be handed over.
var (
	// ErrNoRelease means nothing published fits, which is different from the
	// plugin not existing.
	ErrNoRelease = errors.New("no release matches")
	// ErrReleaseYanked means the exact version asked for was withdrawn. Saying
	// so is the point: a recovery is allowed, but not by accident.
	ErrReleaseYanked = errors.New("that release was withdrawn")
	// ErrClientTooOld means the release states a lower bound this build does
	// not meet.
	ErrClientTooOld = errors.New("this build is older than the release requires")
)

// SelectOptions describes which release a caller wants.
type SelectOptions struct {
	// Version asks for one exact release. Empty takes the default selection.
	Version string
	// ClientVersion is the build that will install it. An empty or unplaceable
	// one satisfies every bound, because refusing to install on a version
	// nobody can compare would break every contributor's checkout.
	ClientVersion string
	// AllowYanked permits a withdrawn release. It exists so a recovery has to
	// say out loud what it is doing.
	AllowYanked bool
	// AllowPrerelease permits a prerelease in the default selection. Naming a
	// prerelease version exactly always works without it.
	AllowPrerelease bool
}

// SelectRelease picks the release a caller should install.
//
// The default is the newest release that is not a prerelease, not withdrawn,
// and whose lower bound this build meets. Every one of those exclusions is
// recoverable by naming a version exactly, which is what keeps the default safe
// without making it a wall.
func SelectRelease(releases []model.PluginRelease, opts SelectOptions) (*model.PluginRelease, error) {
	client := coreplugin.ParseClientVersion(opts.ClientVersion)

	if opts.Version != "" {
		for i := range releases {
			if releases[i].Version != opts.Version {
				continue
			}
			found := releases[i]
			if found.Yanked() && !opts.AllowYanked {
				return nil, fmt.Errorf("%w: %s", ErrReleaseYanked, yankNote(found))
			}
			if err := checkBound(found, client); err != nil {
				return nil, err
			}
			return &found, nil
		}
		return nil, ErrNoRelease
	}

	var best *model.PluginRelease
	var bestVersion coreplugin.Version
	for i := range releases {
		candidate := releases[i]
		if candidate.Yanked() && !opts.AllowYanked {
			continue
		}
		// A release whose version will not parse cannot be ordered against the
		// others. Publishing rejects those, so one here is a record from a
		// build that did not — skipping it is better than ranking it by guess.
		version, err := coreplugin.ParseVersion(candidate.Version)
		if err != nil {
			continue
		}
		if !version.IsRelease() && !opts.AllowPrerelease {
			continue
		}
		if checkBound(candidate, client) != nil {
			continue
		}
		if best == nil || version.Compare(bestVersion) > 0 {
			best, bestVersion = &candidate, version
		}
	}
	if best == nil {
		return nil, ErrNoRelease
	}
	return best, nil
}

// checkBound reports whether this build meets a release's lower bound.
//
// A bound that will not parse is treated as no bound. The value reached the
// catalog through a publish that validated it, so an unreadable one is a record
// this build cannot interpret rather than a reason to refuse an install.
func checkBound(release model.PluginRelease, client coreplugin.ClientVersion) error {
	if release.MinBuildmaxVersion == "" {
		return nil
	}
	min, err := coreplugin.ParseVersion(release.MinBuildmaxVersion)
	if err != nil {
		return nil
	}
	if !client.Satisfies(min) {
		return fmt.Errorf("%w: %s needs BuildMax %s or newer",
			ErrClientTooOld, release.Version, release.MinBuildmaxVersion)
	}
	return nil
}

func yankNote(release model.PluginRelease) string {
	if release.YankedReason == "" {
		return release.Version
	}
	return release.Version + ": " + release.YankedReason
}
