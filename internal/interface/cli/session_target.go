package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

// sessionTarget is what a run continues and where it runs.
//
// The two travel together because resuming decides both: a session belongs to
// one Project, and continuing it somewhere that resolves to a different Project
// is not the same conversation carried on, it is a conversation moved.
type sessionTarget struct {
	SessionID string
	Workspace string
}

// resolveSessionTarget settles --continue and --resume against the local
// Project the workspace resolves to.
//
// --continue means the newest session in *this* Project, not the newest session
// on the machine: the global answer was almost never the one wanted, since it
// followed whichever repository was touched last. --resume stays a global
// lookup by id, but the Project recorded on the session is authoritative about
// where it may be continued. See docs/design/local-project-memory.md §11.2.
func resolveSessionTarget(ctx context.Context, resumeID string, cont bool, workspace string, workspaceGiven bool) (sessionTarget, error) {
	target := sessionTarget{SessionID: resumeID, Workspace: workspace}
	switch {
	case resumeID != "":
		return resolveResumeTarget(ctx, target, workspaceGiven)
	case cont:
		return resolveContinueTarget(ctx, target)
	default:
		return target, nil
	}
}

// resolveContinueTarget picks the newest session of the Project the workspace
// resolves to.
func resolveContinueTarget(ctx context.Context, target sessionTarget) (sessionTarget, error) {
	project, err := currentProject(ctx, target.Workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot tell which project this directory belongs to: %v\n", err)
		return sessionTarget{}, err
	}

	list, err := agentapp.NewSessionManager(config.SessionsDir()).List()
	if err != nil {
		slog.Error("load session list failed", "err", err)
		return sessionTarget{}, fmt.Errorf("load session list: %w", err)
	}
	last := latestSessionItem(filterByProject(list, project.ID))
	if last == nil {
		fmt.Fprintf(os.Stderr, "no sessions yet in %s; start one with -p PROMPT or the TUI\n", project.Name)
		return sessionTarget{}, fmt.Errorf("no sessions to continue in project %s", project.ID)
	}
	slog.Info("continue with last session", "id", last.ID, "project_id", project.ID)
	target.SessionID = last.ID
	return target, nil
}

// resolveResumeTarget checks that an explicitly named session may be continued
// here, and picks the directory to continue it in.
func resolveResumeTarget(ctx context.Context, target sessionTarget, workspaceGiven bool) (sessionTarget, error) {
	loaded, err := agentapp.NewSessionManager(config.SessionsDir()).Load(target.SessionID, session.LoadMetaOnly)
	if err != nil {
		// Not this function's refusal to make: an unreadable or unknown session
		// is reported where the session is opened, with the message that path
		// already gives. Resuming proceeds and fails there.
		slog.Warn("resume: could not read session metadata", "id", target.SessionID, "err", err)
		return target, nil
	}
	// A session from before Projects existed, or one a worker wrote, belongs to
	// no local Project. Inferring one from the directory it is being resumed in
	// would attach it -- and later, another Project's memory -- by path
	// coincidence, which is the failure this design exists to remove.
	if loaded.Meta.ProjectID == "" {
		return target, nil
	}

	if !workspaceGiven && loaded.Meta.Workspace != "" {
		if info, statErr := os.Stat(loaded.Meta.Workspace); statErr == nil && info.IsDir() {
			// Resume where the conversation was, rather than wherever the
			// terminal happens to be. Nothing needs re-checking: the recorded
			// root is what produced this session's Project.
			target.Workspace = loaded.Meta.Workspace
			return target, nil
		}
	}

	here, err := currentProject(ctx, target.Workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot tell which project this directory belongs to: %v\n", err)
		return sessionTarget{}, err
	}
	if here.ID == loaded.Meta.ProjectID {
		return target, nil
	}

	// Refused rather than allowed with a warning: the session would go on to
	// read this Project's memory and record its work here, and neither is
	// undone by noticing afterwards.
	owner := projectLabel(ctx, loaded.Meta.ProjectID)
	err = fmt.Errorf("session %s belongs to project %s, not %s (%s)",
		target.SessionID, owner, here.Name, here.ID)
	fmt.Fprintf(os.Stderr, "%v\n\nResume it from a directory of that project, or start a new session here.\n", err)
	return sessionTarget{}, err
}

// currentProject resolves workspace to its Project, registering one if this is
// the first run there.
func currentProject(ctx context.Context, workspace string) (localproject.Project, error) {
	return agentapp.NewProjectManager(config.ProjectsDir()).Resolve(ctx, workspace)
}

// projectLabel names a Project for a message. A Project that cannot be read is
// named by its id alone: the point of the message is to tell two Projects
// apart, and that still does.
func projectLabel(ctx context.Context, id string) string {
	p, err := agentapp.NewProjectManager(config.ProjectsDir()).Store().Get(ctx, id)
	if err != nil || p.Name == "" {
		if errors.Is(err, localproject.ErrNotFound) {
			return id + " (no longer on this machine)"
		}
		return id
	}
	return fmt.Sprintf("%s (%s)", p.Name, id)
}

// filterByProject keeps the sessions belonging to projectID. A projectless
// session never matches: it is not evidence of belonging anywhere, and treating
// it as a match would put a worker's or a pre-Project session into a picker
// scoped to this repository.
func filterByProject(entries []session.ItemSummary, projectID string) []session.ItemSummary {
	if projectID == "" {
		return nil
	}
	out := make([]session.ItemSummary, 0, len(entries))
	for _, e := range entries {
		if e.ProjectID == projectID {
			out = append(out, e)
		}
	}
	return out
}

func latestSessionItem(entries []session.ItemSummary) *session.ItemSummary {
	var best *session.ItemSummary
	for i := range entries {
		if best == nil || entries[i].CreatedAt.After(best.CreatedAt) {
			best = &entries[i]
		}
	}
	return best
}
