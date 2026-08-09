package store

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNotFound is returned when a requested key does not exist in the store.
var ErrNotFound = errors.New("key not found")

// Store is a persistent key-value store.
type Store interface {
	Set(key, value string) error
	Get(key string) (string, error)
	Delete(key string) error
	Exists(key string) bool
}

// FileStore implements Store using the local filesystem.
// Each key is stored as a file named after the key inside Dir.
type FileStore struct {
	Dir string
}

// Compile-time check that FileStore satisfies Store.
var _ Store = (*FileStore)(nil)

// Set writes value to a file named key inside f.Dir.
func (f *FileStore) Set(key, value string) error {
	// TODO: implement — write value to filepath.Join(f.Dir, key)
	return errors.New("not implemented")
}

// Get reads and returns the file contents for key.
// Returns ErrNotFound if the key does not exist.
func (f *FileStore) Get(key string) (string, error) {
	// TODO: implement — read filepath.Join(f.Dir, key); map os.ErrNotExist to ErrNotFound
	return "", errors.New("not implemented")
}

// Delete removes the file for key.
// Returns ErrNotFound if the key does not exist.
func (f *FileStore) Delete(key string) error {
	// TODO: implement — remove filepath.Join(f.Dir, key); map os.ErrNotExist to ErrNotFound
	return errors.New("not implemented")
}

// Exists reports whether key has been stored and not deleted.
func (f *FileStore) Exists(key string) bool {
	// TODO: implement — check if filepath.Join(f.Dir, key) exists
	return false
}

// suppress unused import warning until implementation is added
var _ = filepath.Join
var _ = os.ReadFile
