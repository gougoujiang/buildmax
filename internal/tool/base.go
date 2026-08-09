package tool

import (
	"errors"
	"os"

	"github.com/gougoujiang/buildmax/internal/util"
)

// workspaceTool is embedded by tools that operate on files under a workspace root.
type workspaceTool struct{ root string }

// resolveFilePath resolves path under root. Returns an error if the path escapes
// the root, does not exist, is not accessible, or is a directory.
func (w workspaceTool) resolveFilePath(path string) (string, error) {
	resolved, err := util.ResolvePath(w.root, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", normalizeOSError(err)
	}
	if info.IsDir() {
		return "", errors.New("path is a directory, not a file")
	}
	return resolved, nil
}

// resolveDirPath resolves path under root. Returns an error if the path escapes
// the root, does not exist, is not accessible, or is not a directory.
func (w workspaceTool) resolveDirPath(path string) (string, error) {
	resolved, err := util.ResolvePath(w.root, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", normalizeOSError(err)
	}
	if !info.IsDir() {
		return "", errors.New("path is not a directory")
	}
	return resolved, nil
}

// resolveAnyPath resolves path under root and reports whether the target is a file
// (true) or directory (false). Returns an error if the path escapes root or does
// not exist.
func (w workspaceTool) resolveAnyPath(path string) (resolved string, isFile bool, err error) {
	resolved, err = util.ResolvePath(w.root, path)
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", false, normalizeOSError(err)
	}
	return resolved, !info.IsDir(), nil
}

// normalizeOSError converts common OS errors to concise user-facing messages.
func normalizeOSError(err error) error {
	if os.IsNotExist(err) {
		return errors.New("file not found")
	}
	if os.IsPermission(err) {
		return errors.New("permission denied")
	}
	return err
}
