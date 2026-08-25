package objectstore

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// LocalFSPluginPackageStorage keeps published packages on the server's disk.
// It is what a single-machine deployment uses instead of an S3 bucket.
type LocalFSPluginPackageStorage struct {
	root string
}

// NewLocalFSPluginPackageStorage returns storage rooted at dir.
func NewLocalFSPluginPackageStorage(dir string) *LocalFSPluginPackageStorage {
	return &LocalFSPluginPackageStorage{root: dir}
}

func (s *LocalFSPluginPackageStorage) path(key string) (string, error) {
	clean, err := CleanRelPath(key)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, filepath.FromSlash(clean)), nil
}

// Put writes bytes through a temporary file and renames them into place.
//
// Without that, an upload cut halfway leaves a short file at a
// content-addressed key — a package whose name says what it should contain and
// whose bytes no longer do.
func (s *LocalFSPluginPackageStorage) Put(ctx context.Context, key string, r io.Reader) error {
	full, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, full)
}

// Open returns the package and its size, or ErrNotFound.
func (s *LocalFSPluginPackageStorage) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	full, err := s.path(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// Exists reports whether the package is already stored.
func (s *LocalFSPluginPackageStorage) Exists(ctx context.Context, key string) (bool, error) {
	full, err := s.path(key)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(full); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// PackageKey returns the object key for one release's bytes. The layout is
// this package's; see PluginPackageKey for why it is content-addressed.
func (s *LocalFSPluginPackageStorage) PackageKey(prefix, pluginName, digest string) (string, error) {
	return PluginPackageKey(prefix, pluginName, digest)
}
