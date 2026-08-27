package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/util"
)

// WriteFile writes content to a local file under a workspace root.
type WriteFile struct {
	workspaceTool
}

// NewWriteFile creates a WriteFile tool that writes files under the given workspace root.
func NewWriteFile(ws util.Workspace) *WriteFile {
	return &WriteFile{workspaceTool{ws: ws}}
}

// Name returns the tool name for the LLM.
// Access implements llm.AccessDeclarer.
func (w *WriteFile) Access(_ map[string]any) llm.Access { return llm.AccessWrite }

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

// CheckArgs implements llm.ArgChecker. Writing to a sensitive file (credentials, private keys)
// triggers Ask so the user can confirm intent in interactive sessions.
func (w *WriteFile) CheckArgs(args map[string]any) llm.ToolAction {
	if path, ok := args["file_path"].(string); ok && isSensitivePath(path) {
		return llm.ToolActionAsk
	}
	return llm.ToolActionAllow
}

// Execute writes args["content"] to the file at args["file_path"] if the path is under the tool's root.
// Creates parent directories if needed; overwrites if the file exists. Returns a short success message or error.
func (w *WriteFile) Execute(ctx context.Context, args map[string]any) (string, error) {
	filePath, err := parseRequiredString(args, "file_path")
	if err != nil {
		return "", err
	}

	// content is required and must be a string, but may be empty (clearing a file is allowed).
	contentVal, ok := args["content"]
	if !ok {
		return "", errors.New("missing content")
	}
	content, ok := contentVal.(string)
	if !ok {
		return "", errors.New("content must be a string")
	}

	resolved, err := util.ResolvePath(w.root(), filePath)
	if err != nil {
		return "", err
	}

	// If path already exists it must be a file, not a directory.
	if info, err := os.Stat(resolved); err == nil && info.IsDir() {
		return "", errors.New("path is a directory, not a file")
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
		return "", normalizeOSError(err)
	}

	if err := os.WriteFile(resolved, []byte(content), 0644); err != nil {
		return "", normalizeOSError(err)
	}

	return "File written successfully.", nil
}
