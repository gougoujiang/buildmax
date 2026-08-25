package objectstore

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	"io"
	"os"
	"path/filepath"
)

const artifactContentFilename = "content"

// LocalFSArtifactStorage implements ArtifactStorage on the local filesystem.
type LocalFSArtifactStorage struct {
	artifactDir func(teamID, artifactID string) string
}

// NewLocalFSArtifactStorage returns an ArtifactStorage rooted by the given
// directory function, which maps a team and artifact ID to a directory holding
// that artifact's content file.
func NewLocalFSArtifactStorage(artifactDir func(teamID, artifactID string) string) *LocalFSArtifactStorage {
	return &LocalFSArtifactStorage{artifactDir: artifactDir}
}

func (s *LocalFSArtifactStorage) path(ref coreartifact.Ref) string {
	return filepath.Join(s.artifactDir(ref.TeamID, ref.ArtifactID), artifactContentFilename)
}

func (s *LocalFSArtifactStorage) PutArtifact(_ context.Context, ref coreartifact.Ref, r io.Reader) (string, error) {
	full := s.path(ref)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(full)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return full, nil
}

func (s *LocalFSArtifactStorage) OpenArtifact(_ context.Context, ref coreartifact.Ref) (io.ReadCloser, error) {
	f, err := os.Open(s.path(ref))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apierr.ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

// RemoveArtifact removes the content file and the directory that held it, so a
// deleted artifact leaves no empty shell behind for an operator to wonder about.
func (s *LocalFSArtifactStorage) RemoveArtifact(_ context.Context, ref coreartifact.Ref) error {
	if err := os.Remove(s.path(ref)); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = os.Remove(s.artifactDir(ref.TeamID, ref.ArtifactID))
	return nil
}
