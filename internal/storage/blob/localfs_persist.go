package blob

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// LocalFSPersistStorage implements PersistStorage using the local filesystem.
type LocalFSPersistStorage struct {
	persistRoot func(workspaceID string) string
}

// NewLocalFSPersistStorage returns a PersistStorage that uses the given root function per workspace.
func NewLocalFSPersistStorage(persistRoot func(workspaceID string) string) *LocalFSPersistStorage {
	return &LocalFSPersistStorage{persistRoot: persistRoot}
}

// Put writes one file at relPath under the workspace's persist root.
func (s *LocalFSPersistStorage) Put(ctx context.Context, workspaceID string, relPath string, r io.Reader) error {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return err
	}
	root := s.persistRoot(workspaceID)
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
func (s *LocalFSPersistStorage) Get(ctx context.Context, workspaceID string, relPath string) ([]byte, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return nil, err
	}
	root := s.persistRoot(workspaceID)
	fullPath := filepath.Join(root, filepath.FromSlash(clean))
	return os.ReadFile(fullPath)
}

// ListFiles returns all file relative paths under the workspace persist root (files only).
func (s *LocalFSPersistStorage) ListFiles(ctx context.Context, workspaceID string) ([]string, error) {
	root := s.persistRoot(workspaceID)
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

// PutChatGlobal is a no-op for local FS (chat run global files already live on worker disk).
func (s *LocalFSPersistStorage) PutChatGlobal(ctx context.Context, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error {
	return nil
}

// GetChatGlobal returns ErrNotFound; chat run global files are not in the persist root for local_fs (caller uses local path).
func (s *LocalFSPersistStorage) GetChatGlobal(ctx context.Context, workspaceID, chatID, chatRunID, relPath string) ([]byte, error) {
	return nil, ErrNotFound
}

// PutChatRunArtifacts is a no-op for local FS (run artifacts already live on worker disk).
func (s *LocalFSPersistStorage) PutChatRunArtifacts(ctx context.Context, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error {
	return nil
}

// GetChatRunArtifacts returns ErrNotFound; run artifacts are not in the persist root for local_fs (caller uses local path).
func (s *LocalFSPersistStorage) GetChatRunArtifacts(ctx context.Context, workspaceID, chatID, chatRunID, relPath string) ([]byte, error) {
	return nil, ErrNotFound
}

// MaterializeToDir copies all persistent files from the workspace into dstDir.
// If the persist root does not exist or is empty, no error (empty dst).
func (s *LocalFSPersistStorage) MaterializeToDir(ctx context.Context, workspaceID string, dstDir string) error {
	root := s.persistRoot(workspaceID)
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
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
