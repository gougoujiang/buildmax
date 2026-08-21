package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// EditFile performs exact string replacements in a file under a workspace root.
type EditFile struct {
	workspaceTool
}

// NewEditFile creates an EditFile tool that edits files under the given workspace root.
func NewEditFile(workspaceRoot string) *EditFile {
	return &EditFile{workspaceTool{root: workspaceRoot}}
}

// Name returns the tool name for the LLM.
// Access implements llm.AccessDeclarer.
func (e *EditFile) Access(_ map[string]any) llm.Access { return llm.AccessWrite }

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

// CheckArgs implements llm.ArgChecker. Editing a sensitive file (credentials, private keys)
// triggers Ask so the user can confirm intent in interactive sessions.
func (e *EditFile) CheckArgs(args map[string]any) llm.ToolAction {
	if path, ok := args["file_path"].(string); ok && isSensitivePath(path) {
		return llm.ToolActionAsk
	}
	return llm.ToolActionAllow
}

// Execute performs string replacement(s) in the file at args["file_path"].
// Reads the file, validates old_string uniqueness when replace_all=false, performs replacement(s), and writes back.
func (e *EditFile) Execute(ctx context.Context, args map[string]any) (string, error) {
	filePath, err := parseRequiredString(args, "file_path")
	if err != nil {
		return "", err
	}
	oldString, err := parseRequiredStringRaw(args, "old_string")
	if err != nil {
		return "", err
	}

	// new_string is required and must be a string, but may be empty (to delete content).
	newVal, ok := args["new_string"]
	if !ok {
		return "", errors.New("missing new_string")
	}
	newString, ok := newVal.(string)
	if !ok {
		return "", errors.New("new_string must be a string")
	}

	replaceAll := parseOptionalBool(args, "replace_all", false)

	resolved, err := e.resolveFilePath(filePath)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", normalizeOSError(err)
	}

	// Normalize \r\n → \n so line-ending differences don't cause mismatches.
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	oldString = strings.ReplaceAll(oldString, "\r\n", "\n")
	newString = strings.ReplaceAll(newString, "\r\n", "\n")

	count := strings.Count(content, oldString)

	if !replaceAll {
		if count == 0 {
			return "", errors.New("old_string not found")
		}
		if count > 1 {
			return "", errors.New("old_string is not unique; use replace_all=true to replace all occurrences or provide more context to make old_string unique")
		}
	}

	var modified string
	if replaceAll {
		modified = strings.ReplaceAll(content, oldString, newString)
	} else {
		modified = strings.Replace(content, oldString, newString, 1)
	}

	if err := os.WriteFile(resolved, []byte(modified), 0644); err != nil {
		return "", normalizeOSError(err)
	}

	if replaceAll {
		return fmt.Sprintf("File edited successfully. Replaced %d occurrence(s).", count), nil
	}
	return "File edited successfully. Replaced 1 occurrence.", nil
}
