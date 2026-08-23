package plugin

import (
	"context"
	"errors"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/model"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
)

// ErrExecutableContent means the release contributes hooks or MCP servers.
//
// Phase D1 refuses those unconditionally. Phase D2 replaces this with the
// operator's unattended-eligibility flag, which is the check that decides
// whether a program may run where nobody is present; until that flag exists
// there is nothing to check it against, and activating anyway would put a
// team's runs past a gate the deployment has not built.
var ErrExecutableContent = errors.New("this release contributes hooks or MCP servers, which cannot be activated for background runs yet")

// ErrNotActivated means an agent named a plugin its team has not activated and
// the team curates its own list. In an open-mode team the same name activates
// the plugin instead; see docs/design/plugin-team-distribution.md §4.1.
var ErrNotActivated = errors.New("this team has not activated this plugin")

// ErrNoActivatableRelease means the catalog has the plugin but nothing this
// team could be pinned to — every release is yanked, a prerelease, or refused
// for the reason ErrExecutableContent gives.
var ErrNoActivatableRelease = errors.New("this plugin has no release that can be activated")

// ErrInvalidCuration means the requested curation mode is not one of the two.
var ErrInvalidCuration = errors.New("unknown plugin curation mode")

// ActivateInput pins a release for a team. Version empty means the newest
// activatable release, which is what a curated activation from Portal sends
// when the admin did not pick one.
type ActivateInput struct {
	TeamID     string
	PluginName string
	Version    string
	ActorID    string
}

// Activate pins a release for a team's background runs.
//
// It is the curated path: a person chose this plugin. The open-mode path is
// ResolveSelection, which activates as a side effect of an agent naming it and
// records that difference in the row's origin.
func (s *Service) Activate(ctx context.Context, in ActivateInput) (*model.PluginActivation, error) {
	release, err := s.activatableRelease(ctx, in.PluginName, in.Version)
	if err != nil {
		return nil, err
	}
	return s.pin(ctx, in.TeamID, *release, model.PluginActivationCurated, in.ActorID)
}

// MovePin repoints a team's activation at another release.
//
// It is separate from Activate because it is the action a capability report is
// read before: the bytes change, so what the team accepted changes with them.
// The new release passes the same content check a first activation does, which
// is what stops a plugin whose next version adds a hook from arriving as an
// update.
func (s *Service) MovePin(ctx context.Context, in ActivateInput) (*model.PluginActivation, error) {
	release, err := s.activatableRelease(ctx, in.PluginName, in.Version)
	if err != nil {
		return nil, err
	}
	activation, err := s.Activations.MovePluginActivationPin(ctx, model.MovePluginActivationPinInput{
		TeamID:     in.TeamID,
		PluginName: in.PluginName,
		Version:    release.Version,
		Digest:     release.Digest,
		ActorID:    in.ActorID,
	})
	if err != nil {
		return nil, err
	}
	s.recordActivation(ctx, in.ActorID, in.TeamID, model.AuditPluginPinMoved, *activation)
	return activation, nil
}

// SetActivationEnabled suspends or resumes an activation without losing the
// pin. Suspending fails the runs of the agents that name the plugin; that is
// intended, and it is why this is not a delete.
func (s *Service) SetActivationEnabled(ctx context.Context, teamID, pluginName string, enabled bool, actorID string) (*model.PluginActivation, error) {
	activation, err := s.Activations.SetPluginActivationEnabled(ctx, teamID, pluginName, enabled, actorID)
	if err != nil {
		return nil, err
	}
	action := model.AuditPluginSuspended
	if enabled {
		action = model.AuditPluginResumed
	}
	s.recordActivation(ctx, actorID, teamID, action, *activation)
	return activation, nil
}

// ListActivations returns a team's activations, suspended ones included.
func (s *Service) ListActivations(ctx context.Context, teamID string) ([]model.PluginActivation, error) {
	return s.Activations.ListPluginActivations(ctx, teamID)
}

// ResolveSelection turns the plugin names an agent definition carries into the
// activations that back them, activating what the team's mode allows.
//
// It is the one seam the agent write path calls, because the answer to "may
// this agent name this plugin" is the team's curation mode and nothing the
// caller can work out for itself. In curated mode an unactivated name is
// refused; in open mode it activates the newest activatable release and
// attributes that to the person saving the agent.
//
// A suspended activation is returned rather than refused: the write is not
// where that fails. A run resolving the same name is (§5.3), so refusing here
// would stop somebody editing an agent to remove the plugin that is failing it.
func (s *Service) ResolveSelection(ctx context.Context, teamID string, names []string, actorID string) ([]model.PluginActivation, error) {
	team, err := s.Teams.GetTeam(ctx, teamID)
	if err != nil {
		return nil, err
	}
	if team == nil {
		return nil, model.ErrNotFound
	}
	curation := model.NormalizePluginCuration(string(team.PluginCuration))

	out := make([]model.PluginActivation, 0, len(names))
	for _, name := range names {
		activation, err := s.Activations.GetPluginActivation(ctx, teamID, name)
		if err != nil {
			return nil, err
		}
		if activation != nil {
			out = append(out, *activation)
			continue
		}
		if curation == model.PluginCurationCurated {
			return nil, fmt.Errorf("%w: %s", ErrNotActivated, name)
		}
		created, err := s.autoActivate(ctx, teamID, name, actorID)
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
func (s *Service) autoActivate(ctx context.Context, teamID, pluginName, actorID string) (*model.PluginActivation, error) {
	release, err := s.activatableRelease(ctx, pluginName, "")
	if err != nil {
		return nil, err
	}
	activation, err := s.pin(ctx, teamID, *release, model.PluginActivationAutomatic, actorID)
	if errors.Is(err, model.ErrPluginAlreadyActivated) {
		// Two agents saved at once, both naming the plugin. The row the other
		// write created is the answer, and it is the same pin this one would
		// have made.
		return s.Activations.GetPluginActivation(ctx, teamID, pluginName)
	}
	return activation, err
}

func (s *Service) pin(ctx context.Context, teamID string, release model.PluginRelease, origin model.PluginActivationOrigin, actorID string) (*model.PluginActivation, error) {
	activation, err := s.Activations.ActivatePlugin(ctx, model.ActivatePluginInput{
		TeamID:     teamID,
		PluginName: release.PluginName,
		Version:    release.Version,
		Digest:     release.Digest,
		Origin:     origin,
		ActorID:    actorID,
	})
	if err != nil {
		return nil, err
	}
	s.recordActivation(ctx, actorID, teamID, model.AuditPluginActivated, *activation)
	return activation, nil
}

// activatableRelease resolves the release a pin will name.
//
// An explicit version is taken as given and checked; an empty one selects the
// newest release this team could be pinned to. Both go through the same content
// check, so "activate" and "update" cannot disagree about what a team may run.
func (s *Service) activatableRelease(ctx context.Context, pluginName, version string) (*model.PluginRelease, error) {
	if version != "" {
		release, err := s.Catalog.GetPluginRelease(ctx, pluginName, version)
		if err != nil {
			return nil, err
		}
		if release == nil {
			return nil, model.ErrNotFound
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
	var best *model.PluginRelease
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
func checkActivatable(release model.PluginRelease) error {
	if len(release.Inspection.Hooks) > 0 || len(release.Inspection.MCP) > 0 {
		return fmt.Errorf("%w: %s@%s", ErrExecutableContent, release.PluginName, release.Version)
	}
	return nil
}

func (s *Service) recordActivation(ctx context.Context, actorID, teamID, action string, a model.PluginActivation) {
	s.Audit.Record(ctx, model.AuditEvent{
		TeamID:     teamID,
		ActorType:  model.AuditActorUser,
		ActorID:    actorID,
		Action:     action,
		TargetType: "plugin",
		TargetID:   releaseTarget(a.PluginName, a.Version),
		Detail:     releaseDetail(a.PluginName, a.Version, a.Digest),
	})
}

// SetCuration records who fills a team's plugin activation list.
func (s *Service) SetCuration(ctx context.Context, teamID string, mode model.PluginCuration, actorID string) error {
	if !model.ValidPluginCuration(mode) {
		return fmt.Errorf("%w: %q", ErrInvalidCuration, mode)
	}
	if err := s.Teams.SetTeamPluginCuration(ctx, teamID, mode); err != nil {
		return err
	}
	s.Audit.Record(ctx, model.AuditEvent{
		TeamID:     teamID,
		ActorType:  model.AuditActorUser,
		ActorID:    actorID,
		Action:     model.AuditTeamPluginCuration,
		TargetType: "team",
		TargetID:   teamID,
		Detail:     string(mode),
	})
	return nil
}
