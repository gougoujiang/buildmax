package db

import (
	"errors"
	"testing"

	coreplugin "github.com/icloudbb/buildmax/internal/core/plugin"
	coretask "github.com/icloudbb/buildmax/internal/core/task"
)

// envEntries is a small ordered set used across the environment tests: one
// baseline entry and one autonomous Task-scoped addition.
func envEntries() []coreplugin.EnvironmentEntry {
	return []coreplugin.EnvironmentEntry{
		{
			PluginName: "search", Version: "1.2.0", Digest: "sha256:" + hex64('a'),
			Installer: coreplugin.EntryInstallerBaseline, Scope: coreplugin.EntryScopeSpace,
		},
		{
			PluginName: "grep", Version: "0.9.1", Digest: "sha256:" + hex64('b'),
			Source:    coreplugin.ReleaseSource{RemoteURL: "https://example.test/grep", Commit: "deadbeef"},
			Installer: coreplugin.EntryInstallerAutonomous, Scope: coreplugin.EntryScopeTask,
		},
	}
}

func TestFinalizePluginEnvironmentIsImmutableAndIdempotent(t *testing.T) {
	s, spaceID, taskID, runID := seedTaskForCheckpoint(t)
	ctx := t.Context()

	entries := envEntries()
	env, err := s.FinalizePluginEnvironment(ctx, coreplugin.CreateEnvironmentInput{
		SpaceID: spaceID, TaskID: taskID, SourceTaskRunID: runID, Entries: entries,
	})
	if err != nil {
		t.Fatalf("finalize environment: %v", err)
	}
	if env.ID == "" || len(env.Entries) != 2 {
		t.Fatalf("finalize returned %+v, want a handle and 2 entries", env)
	}

	// The revision reads back with its entries and provenance intact.
	got, err := s.GetPluginEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("GetPluginEnvironment: %v", err)
	}
	if got.SourceTaskRunID != runID || got.SpaceID != spaceID || got.TaskID != taskID {
		t.Errorf("owners = space %q task %q run %q, want %q/%q/%q",
			got.SpaceID, got.TaskID, got.SourceTaskRunID, spaceID, taskID, runID)
	}
	if got.Entries[1].Installer != coreplugin.EntryInstallerAutonomous ||
		got.Entries[1].Scope != coreplugin.EntryScopeTask ||
		got.Entries[1].Source.Commit != "deadbeef" {
		t.Errorf("autonomous entry did not round-trip: %+v", got.Entries[1])
	}

	// Idempotent: the same run and identical entries return the same revision.
	again, err := s.FinalizePluginEnvironment(ctx, coreplugin.CreateEnvironmentInput{
		SpaceID: spaceID, TaskID: taskID, SourceTaskRunID: runID, Entries: envEntries(),
	})
	if err != nil {
		t.Fatalf("finalize environment (idempotent): %v", err)
	}
	if again.ID != env.ID {
		t.Errorf("idempotent finalize returned %q, want the first revision %q", again.ID, env.ID)
	}

	// Conflict: the same run with different entries never rewrites the accepted
	// revision.
	changed := envEntries()
	changed[1].Version = "9.9.9"
	if _, err := s.FinalizePluginEnvironment(ctx, coreplugin.CreateEnvironmentInput{
		SpaceID: spaceID, TaskID: taskID, SourceTaskRunID: runID, Entries: changed,
	}); !errors.Is(err, coreplugin.ErrEnvironmentConflict) {
		t.Errorf("conflicting finalize error = %v, want ErrEnvironmentConflict", err)
	}
}

func TestFinalizePluginEnvironmentRejectsEmptyAndInvalidEntries(t *testing.T) {
	s, spaceID, taskID, runID := seedTaskForCheckpoint(t)
	ctx := t.Context()

	if _, err := s.FinalizePluginEnvironment(ctx, coreplugin.CreateEnvironmentInput{
		SpaceID: spaceID, TaskID: taskID, SourceTaskRunID: runID, Entries: nil,
	}); !errors.Is(err, coreplugin.ErrEnvironmentEmpty) {
		t.Errorf("empty entries error = %v, want ErrEnvironmentEmpty", err)
	}

	bad := []coreplugin.EnvironmentEntry{{
		PluginName: "search", Version: "1.0.0", Digest: "sha256:" + hex64('a'),
		Installer: "bogus", Scope: coreplugin.EntryScopeSpace,
	}}
	if _, err := s.FinalizePluginEnvironment(ctx, coreplugin.CreateEnvironmentInput{
		SpaceID: spaceID, TaskID: taskID, SourceTaskRunID: runID, Entries: bad,
	}); !errors.Is(err, coreplugin.ErrEnvironmentInvalidEntry) {
		t.Errorf("invalid installer error = %v, want ErrEnvironmentInvalidEntry", err)
	}
}

func TestPluginEnvironmentChainsBaseAndResolvesRunPointer(t *testing.T) {
	s, spaceID, taskID, run1 := seedTaskForCheckpoint(t)
	ctx := t.Context()

	base, err := s.FinalizePluginEnvironment(ctx, coreplugin.CreateEnvironmentInput{
		SpaceID: spaceID, TaskID: taskID, SourceTaskRunID: run1, Entries: envEntries(),
	})
	if err != nil {
		t.Fatalf("finalize base environment: %v", err)
	}

	finishRun(t, s, ctx, run1)
	run2, err := s.CreateTaskRun(ctx, coretask.CreateRunInput{
		TaskID: taskID, Input: "continue", CreatedBy: newTestUser(t, s, "envc2"),
	})
	if err != nil {
		t.Fatalf("CreateTaskRun: %v", err)
	}

	// A successor revision chains to the base.
	next := append(envEntries(), coreplugin.EnvironmentEntry{
		PluginName: "fmt", Version: "2.0.0", Digest: "sha256:" + hex64('c'),
		Installer: coreplugin.EntryInstallerAutonomous, Scope: coreplugin.EntryScopeTask,
	})
	successor, err := s.FinalizePluginEnvironment(ctx, coreplugin.CreateEnvironmentInput{
		SpaceID: spaceID, TaskID: taskID, SourceTaskRunID: run2.ID,
		BaseEnvironmentID: &base.ID, Entries: next,
	})
	if err != nil {
		t.Fatalf("finalize successor environment: %v", err)
	}
	if successor.BaseEnvironmentID == nil || *successor.BaseEnvironmentID != base.ID {
		t.Errorf("successor base = %v, want %q", successor.BaseEnvironmentID, base.ID)
	}

	// A run with no base pointer resolves to nil, the common case.
	if got, err := s.GetRunPluginEnvironmentBase(ctx, run2.ID); err != nil || got != nil {
		t.Errorf("GetRunPluginEnvironmentBase(no base) = (%v, %v), want (nil, nil)", got, err)
	}

	// Once the run's base pointer is set, it resolves to the revision — the same
	// wiring the capability-handoff orchestration will use for a successor run.
	if err := s.db.WithContext(ctx).Exec(
		"UPDATE task_run SET plugin_environment_base_id = (SELECT id FROM plugin_environment WHERE public_id = ?) WHERE public_id = ?",
		base.ID, run2.ID,
	).Error; err != nil {
		t.Fatalf("set run2 environment base: %v", err)
	}
	resolved, err := s.GetRunPluginEnvironmentBase(ctx, run2.ID)
	if err != nil {
		t.Fatalf("GetRunPluginEnvironmentBase: %v", err)
	}
	if resolved == nil || resolved.ID != base.ID {
		t.Errorf("resolved base = %v, want %q", resolved, base.ID)
	}
}
