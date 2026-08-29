package localproject

import (
	"path/filepath"
	"testing"
	"time"
)

// abs builds an absolute path for the platform the test is running on.
//
// A leading separator is not enough on Windows: "\\repo" is rooted but has no
// volume, so it is not absolute and resolves against whichever drive is current.
// Validate is right to reject it, and a test that hand-builds one is testing its
// own path construction rather than the rule.
func abs(t *testing.T, parts ...string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("absolute path for %v: %v", parts, err)
	}
	return p
}

func mustNew(t *testing.T, key Key, name, workspace string) Project {
	t.Helper()
	p, err := New(key, name, workspace, time.Now())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func TestNewGitProjectIsResolvedByItsCommonDir(t *testing.T) {
	common := abs(t, "repo", ".git")
	worktree := abs(t, "repo", "worktrees", "a")
	p := mustNew(t, Key{Kind: KindGit, Locator: common}, "repo", worktree)

	if p.GitCommonDir != common {
		t.Errorf("GitCommonDir = %q, want %q", p.GitCommonDir, common)
	}
	// The point of the pair: identity follows the repository, the working
	// directory does not.
	if got := p.Key(); got.Locator != common {
		t.Errorf("Key().Locator = %q, want the common dir %q", got.Locator, common)
	}
	if p.DefaultWorkspace != worktree {
		t.Errorf("DefaultWorkspace = %q, want the worktree %q", p.DefaultWorkspace, worktree)
	}
}

func TestNewDirectoryProjectIsResolvedByItsRoot(t *testing.T) {
	root := abs(t, "work", "notes")
	p := mustNew(t, Key{Kind: KindDirectory, Locator: root}, "notes", root)

	if p.GitCommonDir != "" {
		t.Errorf("GitCommonDir = %q, want empty for a directory project", p.GitCommonDir)
	}
	if got := p.Key(); got != (Key{Kind: KindDirectory, Locator: root}) {
		t.Errorf("Key() = %+v, want the canonical root", got)
	}
}

// A directory Project's root is its locator, so a caller that passes one
// workspace and a different locator is describing two Projects. New refuses
// rather than persist a record whose Key() answers for a directory it does not
// name.
func TestNewRefusesALocatorThatDisagreesWithTheWorkspace(t *testing.T) {
	_, err := New(
		Key{Kind: KindDirectory, Locator: abs(t, "work", "a")},
		"a",
		abs(t, "work", "b"),
		time.Now(),
	)
	if err == nil {
		t.Fatal("New accepted a directory locator that is not the workspace")
	}
}

func TestValidate(t *testing.T) {
	valid := mustNew(t, Key{Kind: KindGit, Locator: abs(t, "repo", ".git")}, "repo", abs(t, "repo"))

	tests := []struct {
		name    string
		mutate  func(p *Project)
		wantErr bool
	}{
		{name: "as constructed", mutate: func(*Project) {}},
		{name: "no id", mutate: func(p *Project) { p.ID = "" }, wantErr: true},
		{name: "id is not a public id", mutate: func(p *Project) { p.ID = "project-1" }, wantErr: true},
		{name: "no name", mutate: func(p *Project) { p.Name = "  " }, wantErr: true},
		{name: "unknown kind", mutate: func(p *Project) { p.Kind = "workspace" }, wantErr: true},
		{name: "relative workspace", mutate: func(p *Project) { p.DefaultWorkspace = "repo" }, wantErr: true},
		{name: "unclean workspace", mutate: func(p *Project) { p.DefaultWorkspace = abs(t, "repo", "..", "repo") + string(filepath.Separator) }, wantErr: true},
		{name: "git without a common dir", mutate: func(p *Project) { p.GitCommonDir = "" }, wantErr: true},
		{name: "relative common dir", mutate: func(p *Project) { p.GitCommonDir = ".git" }, wantErr: true},
		{
			name:    "directory carrying a common dir",
			mutate:  func(p *Project) { p.Kind = KindDirectory },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.mutate(&p)
			err := p.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestApplyRenameDoesNotChangeIdentity(t *testing.T) {
	common := abs(t, "repo", ".git")
	p := mustNew(t, Key{Kind: KindGit, Locator: common}, "repo", abs(t, "repo"))

	name := "renamed"
	next := Apply(p, Update{Name: &name}, p.CreatedAt.Add(time.Hour))

	if next.ID != p.ID || next.Key() != p.Key() {
		t.Errorf("rename changed identity: %s/%+v -> %s/%+v", p.ID, p.Key(), next.ID, next.Key())
	}
	if next.Name != "renamed" {
		t.Errorf("Name = %q, want renamed", next.Name)
	}
	if !next.UpdatedAt.After(p.UpdatedAt) {
		t.Errorf("UpdatedAt not advanced: %v", next.UpdatedAt)
	}
	// LastUsedAt is recency of use, not of edit: renaming a Project must not
	// reorder the picker.
	if !next.LastUsedAt.Equal(p.LastUsedAt) {
		t.Errorf("LastUsedAt = %v, want it untouched at %v", next.LastUsedAt, p.LastUsedAt)
	}
}

func TestApplyRelinkMovesTheLocator(t *testing.T) {
	p := mustNew(t,
		Key{Kind: KindGit, Locator: abs(t, "old", ".git")},
		"repo", abs(t, "old"))

	moved := abs(t, "new", ".git")
	root := abs(t, "new")
	next := Apply(p, Update{GitCommonDir: &moved, DefaultWorkspace: &root}, time.Now())

	if next.ID != p.ID {
		t.Errorf("relink changed the id: %s -> %s", p.ID, next.ID)
	}
	if next.Key().Locator != moved {
		t.Errorf("Key().Locator = %q, want %q", next.Key().Locator, moved)
	}
	if err := next.Validate(); err != nil {
		t.Errorf("relinked project does not validate: %v", err)
	}
}

func TestNameForWorkspace(t *testing.T) {
	tests := []struct {
		workspace string
		want      string
	}{
		{filepath.Join(string(filepath.Separator), "work", "buildmax"), "buildmax"},
		{filepath.Join(string(filepath.Separator), "work", "buildmax") + string(filepath.Separator), "buildmax"},
		{string(filepath.Separator), "project"},
		{"", "project"},
	}
	for _, tt := range tests {
		if got := NameForWorkspace(tt.workspace); got != tt.want {
			t.Errorf("NameForWorkspace(%q) = %q, want %q", tt.workspace, got, tt.want)
		}
	}
}
