// Package tools provides concrete agent tools (e.g. read_file, write_file).
package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// WriteFile is a tool that writes content to a local file under a root directory.
// It implements the agent.Tool interface.
type WriteFile struct {
	root string // absolute path; all resolved paths must be under this
}

// NewWriteFile creates a WriteFile tool that allows writing files under root.
// If root is empty, the current working directory is used.
// root is normalized and absolutized; an error is returned if it cannot be resolved.
func NewWriteFile(root string) (*WriteFile, error) {
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &WriteFile{root: filepath.Clean(abs)}, nil
}

// Name returns the tool name for the LLM.
func (w *WriteFile) Name() string { return "Write" }

// Description returns a short description so the LLM knows when to use this tool.
func (w *WriteFile) Description() string {
	return "Write or overwrite a local file with the given content. Path must be under the allowed workspace."
}

// Parameters returns the OpenAI-style JSON schema for the tool arguments.
func (w *WriteFile) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write (absolute or relative to the allowed root)",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file (UTF-8)",
			},
		},
		"required": []string{"file_path", "content"},
	}
}

// Execute writes args["content"] to the file at args["file_path"] if the path is under the tool's root.
// Creates parent directories if needed; overwrites if the file exists. Returns a short success message or error.
func (w *WriteFile) Execute(ctx context.Context, args map[string]any) (string, error) {
	vPath, ok := args["file_path"]
	if !ok {
		return "", errors.New("missing file_path")
	}
	filePath, ok := vPath.(string)
	if !ok {
		return "", errors.New("file_path must be a string")
	}
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "", errors.New("file_path is empty")
	}

	vContent, ok := args["content"]
	if !ok {
		return "", errors.New("missing content")
	}
	content, ok := vContent.(string)
	if !ok {
		return "", errors.New("content must be a string")
	}

	// Resolve path against root: join then clean then absolutize
	joined := filepath.Join(w.root, filePath)
	resolved, err := filepath.Abs(filepath.Clean(joined))
	if err != nil {
		return "", err
	}

	// Ensure resolved path is under root (reject path traversal and paths outside root)
	rel, err := filepath.Rel(w.root, resolved)
	if err != nil {
		return "", errors.New("path outside allowed root")
	}
	if rel == ".." || strings.HasPrefix(rel, "..") {
		return "", errors.New("path outside allowed root")
	}
	// On Windows, Rel can return an absolute path when roots differ; ensure resolved is under root.
	cleanRoot := filepath.Clean(w.root)
	resolvedClean := filepath.Clean(resolved)
	if resolvedClean != cleanRoot && !strings.HasPrefix(resolvedClean, cleanRoot+string(filepath.Separator)) {
		return "", errors.New("path outside allowed root")
	}

	// If path exists, it must be a file (not a directory)
	info, err := os.Stat(resolved)
	if err == nil {
		if info.IsDir() {
			return "", errors.New("path is a directory, not a file")
		}
	}
	// err != nil: file may not exist, which is fine

	// Create parent directory if needed
	parent := filepath.Dir(resolved)
	if err := os.MkdirAll(parent, 0755); err != nil {
		if os.IsPermission(err) {
			return "", errors.New("permission denied")
		}
		return "", err
	}

	data := []byte(content)
	if err := os.WriteFile(resolved, data, 0644); err != nil {
		if os.IsPermission(err) {
			return "", errors.New("permission denied")
		}
		return "", err
	}

	return "File written successfully.", nil
}
