package objectstore

import (
	"bytes"
	"context"
	"errors"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactObjectKeyIsDisjointFromTheOtherKeySpaces(t *testing.T) {
	ref := coreartifact.Ref{TeamID: "tm_1", ArtifactID: "jsyt7at6cjfr33d73mta"}
	got := ArtifactObjectKey("workspaces", ref)
	if got != "workspaces/teams/tm_1/artifacts/jsyt7at6cjfr33d73mta/content" {
		t.Fatalf("key = %q", got)
	}
	// The run-output tree is keyed by the creating user and the home tree by
	// the team itself. A literal "teams" segment cannot collide with either,
	// because no id is that string.
	runOutput, err := RunOutputFileKey("workspaces", "u_1", "c_1", "t_1", "r_1", "out.md")
	if err != nil {
		t.Fatal(err)
	}
	home, err := PersistObjectKey("workspaces", "tm_1", "notes.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range []string{runOutput, home} {
		if strings.HasPrefix(other, got) || strings.HasPrefix(got, other) {
			t.Errorf("artifact key %q overlaps %q", got, other)
		}
	}
}

func TestLocalFSArtifactRoundTrip(t *testing.T) {
	root := t.TempDir()
	s := NewLocalFSArtifactStorage(func(teamID, artifactID string) string {
		return filepath.Join(root, teamID, artifactID)
	})
	ctx := context.Background()
	ref := coreartifact.Ref{TeamID: "tm_1", ArtifactID: "jsyt7at6cjfr33d73mta"}

	key, err := s.PutArtifact(ctx, ref, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if key == "" {
		t.Error("put must report the key it wrote")
	}
	body, err := s.OpenArtifact(ctx, ref)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	_ = body.Close()
	if !bytes.Equal(got, []byte("hello")) {
		t.Errorf("content = %q, want hello", got)
	}
}

func TestLocalFSArtifactMissingContentIsNotFound(t *testing.T) {
	root := t.TempDir()
	s := NewLocalFSArtifactStorage(func(teamID, artifactID string) string {
		return filepath.Join(root, teamID, artifactID)
	})
	_, err := s.OpenArtifact(context.Background(), coreartifact.Ref{TeamID: "tm_1", ArtifactID: "ksyt7at6cjfr33d73mta"})
	if !errors.Is(err, apierr.ErrNotFound) {
		t.Errorf("err = %v, want apierr.ErrNotFound", err)
	}
}

// Removal has to be safe to run twice: it is the crash-recovery path for an
// upload whose record never committed.
func TestLocalFSArtifactRemoveIsRepeatable(t *testing.T) {
	root := t.TempDir()
	s := NewLocalFSArtifactStorage(func(teamID, artifactID string) string {
		return filepath.Join(root, teamID, artifactID)
	})
	ctx := context.Background()
	ref := coreartifact.Ref{TeamID: "tm_1", ArtifactID: "jsyt7at6cjfr33d73mta"}
	if _, err := s.PutArtifact(ctx, ref, strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.RemoveArtifact(ctx, ref); err != nil {
			t.Fatalf("remove %d: %v", i, err)
		}
	}
	if _, err := s.OpenArtifact(ctx, ref); !errors.Is(err, apierr.ErrNotFound) {
		t.Errorf("after removal err = %v, want apierr.ErrNotFound", err)
	}
}
