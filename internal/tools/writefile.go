// Package tools provides concrete agent tools (e.g. read_file, write_file).
package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"buildmax/internal/util"
)

// WriteFile is a tool that writes content to a local file under a workspace root.
// It implements the agent.Tool interface.
type WriteFile struct {
	ws *util.Workspace
}

// NewWriteFile creates a WriteFile tool that writes files under the given workspace.
func NewWriteFile(ws *util.Workspace) *WriteFile {
	return &WriteFile{ws: ws}
}

// Name returns the tool name for the LLM.
func (w *WriteFile) Name() string { return ToolNameWrite }

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
	filePath, err := RequiredString(args, "file_path")
	if err != nil {
		return "", err
	}
	contentVal, ok := args["content"]
	if !ok {
		return "", errors.New("missing content")
	}
	content, ok := contentVal.(string)
	if !ok {
		return "", errors.New("content must be a string")
	}
	// content may be empty (allowed for clearing a file)

	// Resolve path under root (join, clean, boundary check including Windows-safe prefix).
	resolved, err := w.ws.ResolvePath(filePath)
	if err != nil {
		return "", err
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
