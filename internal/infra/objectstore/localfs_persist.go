package objectstore

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// LocalFSPersistStorage implements PersistStorage using the local filesystem.
type LocalFSPersistStorage struct {
	persistRoot func(teamID string) string
}

// NewLocalFSPersistStorage returns a PersistStorage that uses the given root function per team.
func NewLocalFSPersistStorage(persistRoot func(teamID string) string) *LocalFSPersistStorage {
	return &LocalFSPersistStorage{persistRoot: persistRoot}
}

// Put writes one file at relPath under the team's persist root.
func (s *LocalFSPersistStorage) Put(ctx context.Context, teamID string, relPath string, r io.Reader) error {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return err
	}
	root := s.persistRoot(teamID)
	fullPath := filepath.Join(root, filepath.FromSlash(clean))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// Get reads one file. Returns os.ErrNotExist if the file does not exist.
func (s *LocalFSPersistStorage) Get(ctx context.Context, teamID string, relPath string) ([]byte, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return nil, err
	}
	root := s.persistRoot(teamID)
	fullPath := filepath.Join(root, filepath.FromSlash(clean))
	return os.ReadFile(fullPath)
}

// ListFiles returns all file relative paths under the team persist root (files only).
func (s *LocalFSPersistStorage) ListFiles(ctx context.Context, teamID string) ([]string, error) {
	root := s.persistRoot(teamID)
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		out = append(out, rel)
		return nil
	})
	return out, err
}

// PutRunGlobal is a no-op for local FS (task run global files already live on worker disk).
func (s *LocalFSPersistStorage) PutRunGlobal(ctx context.Context, ref RunObjectRef, r io.Reader) error {
	return nil
}

// GetRunGlobal returns ErrNotFound; task run global files are not in the persist root for local_fs (caller uses local path).
func (s *LocalFSPersistStorage) GetRunGlobal(ctx context.Context, ref RunObjectRef) ([]byte, error) {
	return nil, ErrNotFound
}

// PutRunArtifacts is a no-op for local FS (run artifacts already live on worker disk).
func (s *LocalFSPersistStorage) PutRunArtifacts(ctx context.Context, ref RunObjectRef, r io.Reader) error {
	return nil
}

// GetRunArtifacts returns ErrNotFound; run artifacts are not in the persist root for local_fs (caller uses local path).
func (s *LocalFSPersistStorage) GetRunArtifacts(ctx context.Context, ref RunObjectRef) ([]byte, error) {
	return nil, ErrNotFound
}

// MaterializeToDir copies all persistent files from the team into dstDir.
// If the persist root does not exist or is empty, no error (empty dst).
func (s *LocalFSPersistStorage) MaterializeToDir(ctx context.Context, teamID string, dstDir string) error {
	root := s.persistRoot(teamID)
	return copyDirContents(root, dstDir)
}

// copyDirContents copies files and directories from src to dst recursively.
// If src is missing or not a directory, returns nil (no-op).
func copyDirContents(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		destPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, destPath)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	// Close reports write errors that Copy could not see yet, so on the write
	// side it belongs in the return path rather than in a defer.
	return out.Close()
}
