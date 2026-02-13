// Package tools provides concrete agent tools (e.g. read_file, write_file, edit_file).
package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EditFile is a tool that performs exact string replacements in a file under a root directory.
// It implements the agent.Tool interface.
type EditFile struct {
	root string // absolute path; all resolved paths must be under this
}

// NewEditFile creates an EditFile tool that allows editing files under root.
// If root is empty, the current working directory is used.
// root is normalized and absolutized; an error is returned if it cannot be resolved.
func NewEditFile(root string) (*EditFile, error) {
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
	return &EditFile{root: filepath.Clean(abs)}, nil
}

// Name returns the tool name for the LLM.
func (e *EditFile) Name() string { return ToolNameEdit }

// Description returns a short description so the LLM knows when to use this tool.
func (e *EditFile) Description() string {
	return "Performs exact string replacements in files. You must read the file first before editing. When editing text from Read tool output, preserve the exact indentation (tabs/spaces) as it appears AFTER the line number prefix. The edit will fail if old_string is not unique (unless replace_all=true). Use replace_all=true to replace all occurrences or provide more context to make old_string unique."
}

// Parameters returns the OpenAI-style JSON schema for the tool arguments.
func (e *EditFile) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to modify (absolute or relative to the allowed root)",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "The text to replace",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "The text to replace it with (must be different from old_string)",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace all occurrences of old_string (default false)",
			},
		},
		"required": []string{"file_path", "old_string", "new_string"},
	}
}

// Execute performs string replacement(s) in the file at args["file_path"] if the path is under the tool's root.
// Reads the file, validates old_string uniqueness when replace_all=false, performs replacement(s), and writes back.
// Returns a success message with replacement count or error.
func (e *EditFile) Execute(ctx context.Context, args map[string]any) (string, error) {
	// Extract and validate file_path
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

	// Extract and validate old_string
	vOld, ok := args["old_string"]
	if !ok {
		return "", errors.New("missing old_string")
	}
	oldString, ok := vOld.(string)
	if !ok {
		return "", errors.New("old_string must be a string")
	}
	if oldString == "" {
		return "", errors.New("old_string is empty")
	}

	// Extract and validate new_string
	vNew, ok := args["new_string"]
	if !ok {
		return "", errors.New("missing new_string")
	}
	newString, ok := vNew.(string)
	if !ok {
		return "", errors.New("new_string must be a string")
	}

	// Extract replace_all (optional, default false)
	replaceAll := false
	if vReplaceAll, ok := args["replace_all"]; ok && vReplaceAll != nil {
		if b, ok := vReplaceAll.(bool); ok {
			replaceAll = b
		}
		// If not bool, ignore (defaults to false)
	}

	// Resolve path against root: join then clean then absolutize
	joined := filepath.Join(e.root, filePath)
	resolved, err := filepath.Abs(filepath.Clean(joined))
	if err != nil {
		return "", err
	}

	// Ensure resolved path is under root (reject path traversal and paths outside root)
	rel, err := filepath.Rel(e.root, resolved)
	if err != nil {
		return "", errors.New("path outside allowed root")
	}
	if rel == ".." || strings.HasPrefix(rel, "..") {
		return "", errors.New("path outside allowed root")
	}
	// On Windows, Rel can return an absolute path when roots differ; ensure resolved is under root.
	cleanRoot := filepath.Clean(e.root)
	resolvedClean := filepath.Clean(resolved)
	if resolvedClean != cleanRoot && !strings.HasPrefix(resolvedClean, cleanRoot+string(filepath.Separator)) {
		return "", errors.New("path outside allowed root")
	}

	// Verify file exists and is not a directory
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("file not found")
		}
		if os.IsPermission(err) {
			return "", errors.New("permission denied")
		}
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("path is a directory, not a file")
	}

	// Read file content
	data, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsPermission(err) {
			return "", errors.New("permission denied")
		}
		return "", err
	}
	// Normalize \r\n → \n in file content and old/new strings so that
	// line-ending differences (common on Windows) do not cause mismatches.
	// The LLM typically sends strings with \n only, while files on disk
	// may contain \r\n.
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	oldString = strings.ReplaceAll(oldString, "\r\n", "\n")
	newString = strings.ReplaceAll(newString, "\r\n", "\n")

	// Count occurrences of old_string
	count := strings.Count(content, oldString)

	// Validate uniqueness when replace_all=false
	if !replaceAll {
		if count == 0 {
			return "", errors.New("old_string not found")
		}
		if count > 1 {
			return "", errors.New("old_string is not unique; use replace_all=true to replace all occurrences or provide more context to make old_string unique")
		}
	}

	// Perform replacement
	var modified string
	if replaceAll {
		modified = strings.ReplaceAll(content, oldString, newString)
	} else {
		modified = strings.Replace(content, oldString, newString, 1)
	}

	// Write modified content back
	if err := os.WriteFile(resolved, []byte(modified), 0644); err != nil {
		if os.IsPermission(err) {
			return "", errors.New("permission denied")
		}
		return "", err
	}

	// Return success message with replacement count
	if replaceAll {
		return fmt.Sprintf("File edited successfully. Replaced %d occurrence(s).", count), nil
	}
	return "File edited successfully. Replaced 1 occurrence.", nil
}
