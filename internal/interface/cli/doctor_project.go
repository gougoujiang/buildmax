package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"unicode/utf8"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
	"github.com/gougoujiang/buildmax/internal/infra/localprojectstore"
)

// checkProject reports which local Project this directory belongs to, and what
// state its memory is in.
//
// Neither answer is guessable from outside: a Project is a stable id with a
// locator behind it, and the memory document sits under BUILDMAX_HOME rather
// than in the repository. Doctor is where a person finds both. It reports sizes
// and paths, never the document's content -- that is the user's to open, and
// doctor's output is the kind of thing people paste into an issue. See
// docs/design/local-project-memory.md §12.
func checkProject(ctx context.Context, workspace string) []doctorCheck {
	project, err := agentapp.NewProjectManager(config.ProjectsDir()).Lookup(ctx, workspace)
	switch {
	case errors.Is(err, localproject.ErrNotFound):
		// Not a problem to fix. A directory nothing has run in yet has no
		// Project, and doctor is a diagnosis: reporting the state is its job,
		// creating one is the first run's.
		return []doctorCheck{{
			Severity: doctorOK,
			Title:    "project",
			Detail:   "no project registered for this directory yet; the first run here registers one",
		}}
	case errors.Is(err, localproject.ErrDuplicateLocator):
		// Two Projects claiming one locator is not something to work around:
		// either choice silently joins or splits a memory domain.
		return []doctorCheck{{
			Severity: doctorFail,
			Title:    "project",
			Detail:   err.Error(),
			Next: "Two project bundles claim this directory. Remove the one you do not want from " +
				config.ProjectsDir() + " before running here.",
		}}
	case err != nil:
		return []doctorCheck{{
			Severity: doctorWarn,
			Title:    "project",
			Detail:   fmt.Sprintf("cannot tell which project this directory belongs to: %v", err),
			Next:     "Run from a directory BuildMax can read, or pass `--workspace <dir>`.",
		}}
	}

	detail := fmt.Sprintf("%s (%s, %s)", project.Name, project.ID, project.Kind)
	if project.Kind == localproject.KindGit {
		// The locator, so a person can see which repository the id is pinned to
		// and recognize a stale one after the repository moves.
		detail += fmt.Sprintf("\n    repository: %s", project.GitCommonDir)
	}
	return []doctorCheck{
		{Severity: doctorOK, Title: "project", Detail: detail},
		checkProjectMemory(ctx, project),
	}
}

// checkProjectMemory reports whether memory exists, fits its budget, and still
// matches the metadata BuildMax recorded.
func checkProjectMemory(ctx context.Context, project localproject.Project) doctorCheck {
	path := filepath.Join(config.ProjectsDir(), project.ID,
		localprojectstore.MemoryDir, localprojectstore.MemoryFile)

	memory, err := agentapp.NewProjectManager(config.ProjectsDir()).Store().ReadMemory(ctx, project.ID)
	if err != nil {
		return doctorCheck{
			Severity: doctorWarn,
			Title:    "project memory",
			Detail:   fmt.Sprintf("cannot read %s: %v", path, err),
		}
	}
	if memory.Content == "" {
		return doctorCheck{
			Severity: doctorOK,
			Title:    "project memory",
			Detail:   "empty; nothing is remembered for this project yet",
		}
	}

	chars := utf8.RuneCountInString(memory.Content)
	if err := localproject.ValidateMemory(memory.Content); err != nil {
		// Named as a failure because the run silently goes without: a document
		// this size or shape is skipped rather than sent as a prefix, and
		// nothing else tells the user their memory stopped being loaded.
		return doctorCheck{
			Severity: doctorFail,
			Title:    "project memory",
			Detail:   fmt.Sprintf("%s is not usable and is not loaded into runs: %v", path, err),
			Next: fmt.Sprintf("Edit %s down to %d characters of valid UTF-8, or empty it.",
				path, localproject.MaxMemoryChars),
		}
	}

	detail := fmt.Sprintf("revision %d, %d/%d characters\n    %s",
		memory.Meta.Revision, chars, localproject.MaxMemoryChars, path)
	if memory.ManuallyEdited {
		// Not a problem, and worth saying: the file is the authority, so the
		// revision beside it describes an older write than what is loaded.
		return doctorCheck{
			Severity: doctorOK,
			Title:    "project memory",
			Detail:   detail + "\n    edited by hand since BuildMax last wrote it; the file is what runs load",
		}
	}
	return doctorCheck{Severity: doctorOK, Title: "project memory", Detail: detail}
}

// checkDetachedSessions reports sessions naming a Project this machine no
// longer has.
//
// They still open, and resuming one never re-attaches it by guessing: the point
// of listing them is that a person decides. See
// docs/design/local-project-memory.md §15.
func checkDetachedSessions(ctx context.Context) doctorCheck {
	rows, err := agentapp.NewSessionManager(config.SessionsDir()).List()
	if err != nil {
		return doctorCheck{
			Severity: doctorWarn,
			Title:    "sessions",
			Detail:   fmt.Sprintf("cannot list sessions: %v", err),
		}
	}
	store := agentapp.NewProjectManager(config.ProjectsDir()).Store()
	known := map[string]bool{}
	var detached int
	for _, row := range rows {
		if row.ProjectID == "" {
			continue
		}
		exists, seen := known[row.ProjectID]
		if !seen {
			_, getErr := store.Get(ctx, row.ProjectID)
			exists = getErr == nil
			known[row.ProjectID] = exists
		}
		if !exists {
			detached++
		}
	}
	if detached == 0 {
		return doctorCheck{
			Severity: doctorOK,
			Title:    "sessions",
			Detail:   fmt.Sprintf("%d session(s), all attached to a project on this machine", len(rows)),
		}
	}
	return doctorCheck{
		Severity: doctorWarn,
		Title:    "sessions",
		Detail:   fmt.Sprintf("%d of %d session(s) name a project this machine no longer has", detached, len(rows)),
		Next: "They still open with `buildmax --resume <id>`. Nothing re-attaches them automatically, " +
			"because the project a session belongs to is not something to guess.",
	}
}
