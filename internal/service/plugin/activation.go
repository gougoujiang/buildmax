package plugin

import (
	"context"
	"errors"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
)

// ErrExecutableContent means the release contributes hooks or MCP servers.
//
// Phase D1 refuses those unconditionally. Phase D2 replaces this with the
// operator's unattended-eligibility flag, which is the check that decides
// whether a program may run where nobody is present; until that flag exists
// there is nothing to check it against, and activating anyway would put a
// space's runs past a gate the deployment has not built.
var ErrExecutableContent = errors.New("this release contributes hooks or MCP servers, which cannot be activated for background runs yet")

// ErrNotActivated means an agent named a plugin its space has not activated and
// the space curates its own list. In an open-mode space the same name activates
// the plugin instead; see docs/design/plugin-space-distribution.md §4.1.
var ErrNotActivated = errors.New("this space has not activated this plugin")

// ErrNoActivatableRelease means the catalog has the plugin but nothing this
// space could be pinned to — every release is yanked, a prerelease, or refused
// for the reason ErrExecutableContent gives.
var ErrNoActivatableRelease = errors.New("this plugin has no release that can be activated")

// ErrInvalidCuration means the requested curation mode is not one of the two.
var ErrInvalidCuration = errors.New("unknown plugin curation mode")

// ActivateInput pins a release for a space. Version empty means the newest
// activatable release, which is what a curated activation from Portal sends
// when the admin did not pick one.
type ActivateInput struct {
	SpaceID    string
	PluginName string
	Version    string
	ActorID    string
}

// Activate pins a release for a space's background runs.
//
// It is the curated path: a person chose this plugin. The open-mode path is
// ResolveSelection, which activates as a side effect of an agent naming it and
// records that difference in the row's origin.
func (s *Service) Activate(ctx context.Context, in ActivateInput) (*coreplugin.Activation, error) {
	release, err := s.activatableRelease(ctx, in.PluginName, in.Version)
	if err != nil {
		return nil, err
	}
	return s.pin(ctx, in.SpaceID, *release, coreplugin.ActivationCurated, in.ActorID)
}

// MovePin repoints a space's activation at another release.
//
// It is separate from Activate because it is the action a capability report is
// read before: the bytes change, so what the space accepted changes with them.
// The new release passes the same content check a first activation does, which
// is what stops a plugin whose next version adds a hook from arriving as an
// update.
func (s *Service) MovePin(ctx context.Context, in ActivateInput) (*coreplugin.Activation, error) {
	release, err := s.activatableRelease(ctx, in.PluginName, in.Version)
	if err != nil {
		return nil, err
	}
	activation, err := s.Activations.MovePluginActivationPin(ctx, coreplugin.MovePinInput{
		SpaceID:    in.SpaceID,
		PluginName: in.PluginName,
		Version:    release.Version,
		Digest:     release.Digest,
		ActorID:    in.ActorID,
	})
	if err != nil {
		return nil, err
	}
	s.recordActivation(ctx, in.ActorID, in.SpaceID, coreaudit.PluginPinMoved, *activation)
	return activation, nil
}

// SetActivationEnabled suspends or resumes an activation without losing the
// pin. Suspending fails the runs of the agents that name the plugin; that is
// intended, and it is why this is not a delete.
func (s *Service) SetActivationEnabled(ctx context.Context, spaceID, pluginName string, enabled bool, actorID string) (*coreplugin.Activation, error) {
	activation, err := s.Activations.SetPluginActivationEnabled(ctx, spaceID, pluginName, enabled, actorID)
	if err != nil {
		return nil, err
	}
	action := coreaudit.PluginSuspended
	if enabled {
		action = coreaudit.PluginResumed
	}
	s.recordActivation(ctx, actorID, spaceID, action, *activation)
	return activation, nil
}

// ListActivations returns a space's activations, suspended ones included.
func (s *Service) ListActivations(ctx context.Context, spaceID string) ([]coreplugin.Activation, error) {
	return s.Activations.ListPluginActivations(ctx, spaceID)
}

// ResolveSelection turns the plugin names an agent definition carries into the
// activations that back them, activating what the space's mode allows.
//
// It is the one seam the agent write path calls, because the answer to "may
// this agent name this plugin" is the space's curation mode and nothing the
// caller can work out for itself. In curated mode an unactivated name is
// refused; in open mode it activates the newest activatable release and
// attributes that to the person saving the agent.
//
// A suspended activation is returned rather than refused: the write is not
// where that fails. A run resolving the same name is (§5.3), so refusing here
// would stop somebody editing an agent to remove the plugin that is failing it.
func (s *Service) ResolveSelection(ctx context.Context, spaceID string, names []string, actorID string) ([]coreplugin.Activation, error) {
	space, err := s.Spaces.GetSpace(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	if space == nil {
		return nil, apierr.ErrNotFound
	}
	curation := coreplugin.NormalizeCuration(string(space.PluginCuration))

	out := make([]coreplugin.Activation, 0, len(names))
	for _, name := range names {
		activation, err := s.Activations.GetPluginActivation(ctx, spaceID, name)
		if err != nil {
			return nil, err
		}
		if activation != nil {
			out = append(out, *activation)
			continue
		}
		if curation == coreplugin.CurationCurated {
			return nil, fmt.Errorf("%w: %s", ErrNotActivated, name)
		}
		created, err := s.autoActivate(ctx, spaceID, name, actorID)
		if err != nil {
			return nil, err
		}
		out = append(out, *created)
	}
	return out, nil
}

// autoActivate is open mode's activation: caused by an agent naming the plugin,
// attributed to whoever saved that agent, and otherwise identical to a curated
// one — same pin, same digest, same audit event.
func (s *Service) autoActivate(ctx context.Context, spaceID, pluginName, actorID string) (*coreplugin.Activation, error) {
	release, err := s.activatableRelease(ctx, pluginName, "")
	if err != nil {
		return nil, err
	}
	activation, err := s.pin(ctx, spaceID, *release, coreplugin.ActivationAutomatic, actorID)
	if errors.Is(err, coreplugin.ErrAlreadyActivated) {
		// Two agents saved at once, both naming the plugin. The row the other
		// write created is the answer, and it is the same pin this one would
		// have made.
		return s.Activations.GetPluginActivation(ctx, spaceID, pluginName)
	}
	return activation, err
}

func (s *Service) pin(ctx context.Context, spaceID string, release coreplugin.Release, origin coreplugin.ActivationOrigin, actorID string) (*coreplugin.Activation, error) {
	activation, err := s.Activations.ActivatePlugin(ctx, coreplugin.ActivateInput{
		SpaceID:    spaceID,
		PluginName: release.PluginName,
		Version:    release.Version,
		Digest:     release.Digest,
		Origin:     origin,
		ActorID:    actorID,
	})
	if err != nil {
		return nil, err
	}
	s.recordActivation(ctx, actorID, spaceID, coreaudit.PluginActivated, *activation)
	return activation, nil
}

// activatableRelease resolves the release a pin will name.
//
// An explicit version is taken as given and checked; an empty one selects the
// newest release this space could be pinned to. Both go through the same content
// check, so "activate" and "update" cannot disagree about what a space may run.
func (s *Service) activatableRelease(ctx context.Context, pluginName, version string) (*coreplugin.Release, error) {
	if version != "" {
		release, err := s.Catalog.GetPluginRelease(ctx, pluginName, version)
		if err != nil {
			return nil, err
		}
		if release == nil {
			return nil, apierr.ErrNotFound
		}
		if err := checkActivatable(*release); err != nil {
			return nil, err
		}
		return release, nil
	}

	releases, err := s.Catalog.ListPluginReleases(ctx, pluginName)
	if err != nil {
		return nil, err
	}
	var best *coreplugin.Release
	var bestVersion coreplugin.Version
	// refused remembers the newest candidate that was otherwise selectable and
	// only failed the content check. Without it, activating a plugin whose every
	// release contributes a hook would report "no release can be activated",
	// which says nothing about the one thing the admin can act on.
	var refused error
	for i := range releases {
		candidate := releases[i]
		if candidate.Yanked() {
			continue
		}
		parsed, err := coreplugin.ParseVersion(candidate.Version)
		if err != nil || !parsed.IsRelease() {
			// A prerelease is never selected by default, here as at install.
			continue
		}
		if err := checkActivatable(candidate); err != nil {
			refused = err
			continue
		}
		if best == nil || parsed.Compare(bestVersion) > 0 {
			best = &releases[i]
			bestVersion = parsed
		}
	}
	if best == nil {
		if refused != nil {
			return nil, refused
		}
		return nil, ErrNoActivatableRelease
	}
	return best, nil
}

// checkActivatable is Phase D1's gate: a release that starts a process or opens
// a connection is refused until the operator has a way to say it may.
func checkActivatable(release coreplugin.Release) error {
	if len(release.Inspection.Hooks) > 0 || len(release.Inspection.MCP) > 0 {
		return fmt.Errorf("%w: %s@%s", ErrExecutableContent, release.PluginName, release.Version)
	}
	return nil
}

func (s *Service) recordActivation(ctx context.Context, actorID, spaceID, action string, a coreplugin.Activation) {
	s.Audit.Record(ctx, coreaudit.Event{
		SpaceID:    spaceID,
		ActorType:  coreaudit.ActorUser,
		ActorID:    actorID,
		Action:     action,
		TargetType: "plugin",
		TargetID:   releaseTarget(a.PluginName, a.Version),
		Detail:     releaseDetail(a.PluginName, a.Version, a.Digest),
	})
}

// SetCuration records who fills a space's plugin activation list.
func (s *Service) SetCuration(ctx context.Context, spaceID string, mode coreplugin.Curation, actorID string) error {
	if !coreplugin.ValidCuration(mode) {
		return fmt.Errorf("%w: %q", ErrInvalidCuration, mode)
	}
	if err := s.Spaces.SetSpacePluginCuration(ctx, spaceID, mode); err != nil {
		return err
	}
	s.Audit.Record(ctx, coreaudit.Event{
		SpaceID:    spaceID,
		ActorType:  coreaudit.ActorUser,
		ActorID:    actorID,
		Action:     coreaudit.SpacePluginCuration,
		TargetType: "space",
		TargetID:   spaceID,
		Detail:     string(mode),
	})
	return nil
}
