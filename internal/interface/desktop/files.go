package desktop

import (
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// maxFilePreviewBytes bounds a file preview so opening a huge file in the tree
// cannot stall the UI or blow up the payload.
const maxFilePreviewBytes = 512 * 1024

// WorkspaceEntry is one item in a workspace directory listing. Path is
// slash-separated and relative to the workspace root, so the frontend can pass
// it straight back to list the directory's children.
type WorkspaceEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// WorkspaceListing is one directory of a project's workspace, one level deep.
// Dir echoes the normalised directory that was listed ("" is the root). A
// per-directory Error lets the tree show an unreadable folder in place rather
// than failing the whole panel.
type WorkspaceListing struct {
	Dir     string           `json:"dir"`
	Entries []WorkspaceEntry `json:"entries"`
	Error   string           `json:"error,omitempty"`
}

// ListWorkspaceDir lists one directory of a project's workspace so the Desktop
// file tree can expand lazily instead of walking the whole repository. relPath
// is slash-separated and relative to the workspace root; "" lists the root. The
// path is normalised so it can never escape the root.
func (a *App) ListWorkspaceDir(projectID, relPath string) (WorkspaceListing, error) {
	proj, err := projectManager().Store().Get(context.Background(), projectID)
	if err != nil {
		return WorkspaceListing{}, err
	}
	return listWorkspaceDir(proj.DefaultWorkspace, relPath)
}

// WorkspaceFile is a text preview of one workspace file. Binary content is
// reported rather than returned, and long files are truncated to a bound.
type WorkspaceFile struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
	Error     string `json:"error,omitempty"`
}

// ReadWorkspaceFile returns a bounded text preview of one file in a project's
// workspace. relPath is slash-separated and cannot escape the workspace root.
func (a *App) ReadWorkspaceFile(projectID, relPath string) (WorkspaceFile, error) {
	proj, err := projectManager().Store().Get(context.Background(), projectID)
	if err != nil {
		return WorkspaceFile{}, err
	}
	return readWorkspaceFile(proj.DefaultWorkspace, relPath)
}

func readWorkspaceFile(root, relPath string) (WorkspaceFile, error) {
	clean := strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(relPath, "\\", "/")), "/")
	if clean == "" {
		return WorkspaceFile{Path: clean, Error: "no file selected"}, nil
	}
	full := filepath.Join(root, filepath.FromSlash(clean))

	info, err := os.Stat(full)
	if err != nil {
		return WorkspaceFile{Path: clean, Error: err.Error()}, nil
	}
	if info.IsDir() {
		return WorkspaceFile{Path: clean, Error: "not a file"}, nil
	}

	f, err := os.Open(full)
	if err != nil {
		return WorkspaceFile{Path: clean, Error: err.Error()}, nil
	}
	defer f.Close()

	// Read one byte past the cap so a file exactly at the cap is not reported
	// as truncated.
	data, err := io.ReadAll(io.LimitReader(f, maxFilePreviewBytes+1))
	if err != nil {
		return WorkspaceFile{Path: clean, Error: err.Error()}, nil
	}
	truncated := false
	if len(data) > maxFilePreviewBytes {
		data = data[:maxFilePreviewBytes]
		truncated = true
	}
	// A NUL byte is the usual, cheap signal that this is not text to preview.
	if bytes.IndexByte(data, 0) >= 0 {
		return WorkspaceFile{Path: clean, Binary: true}, nil
	}
	return WorkspaceFile{Path: clean, Content: string(data), Truncated: truncated}, nil
}

func listWorkspaceDir(root, relPath string) (WorkspaceListing, error) {
	// Clean against a leading slash so any ".." collapses to the root rather
	// than climbing above it, then drop the slash to get a root-relative path.
	clean := strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(relPath, "\\", "/")), "/")
	full := filepath.Join(root, filepath.FromSlash(clean))

	dirents, err := os.ReadDir(full)
	if err != nil {
		return WorkspaceListing{Dir: clean, Error: err.Error()}, nil
	}

	entries := make([]WorkspaceEntry, 0, len(dirents))
	for _, de := range dirents {
		name := de.Name()
		if name == ".git" {
			continue
		}
		child := name
		if clean != "" {
			child = clean + "/" + name
		}
		entries = append(entries, WorkspaceEntry{Name: name, Path: child, IsDir: de.IsDir()})
	}

	// Directories first, then case-insensitive by name, matching how file
	// browsers order a tree.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	return WorkspaceListing{Dir: clean, Entries: entries}, nil
}
