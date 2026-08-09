package tool

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// ReadFile reads a local file under a workspace root.
type ReadFile struct {
	workspaceTool
}

// NewReadFile creates a ReadFile tool that reads files under the given workspace root.
func NewReadFile(workspaceRoot string) *ReadFile {
	return &ReadFile{workspaceTool{root: workspaceRoot}}
}

// Name returns the tool name for the LLM.
func (r *ReadFile) Name() string { return ToolNameRead }

// DefaultLimit is the default number of lines returned when limit is not specified.
const DefaultLimit = 1000

// Description returns a short description so the LLM knows when to use this tool.
func (r *ReadFile) Description() string {
	return "Read the contents of a local file (or a range of lines). Use this to read source code, config files, or text. For large files, use offset and limit to read part by part (e.g. offset=1 limit=1000 for first 1000 lines). By default returns the first 1000 lines."
}

// Parameters returns the OpenAI-style JSON schema for the tool arguments.
func (r *ReadFile) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read (absolute or relative to the allowed root)",
			},
			"offset": map[string]any{
				"type":        "number",
				"description": "1-based line number to start reading from (default 1). Use with limit to read large files in chunks.",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "Maximum number of lines to return (default 1000). Use with offset to read large files part by part.",
			},
		},
		"required": []string{"file_path"},
	}
}

// CheckArgs implements llm.ArgChecker. Reading a sensitive file (credentials, private keys)
// triggers Ask so the user can confirm intent in interactive sessions.
func (r *ReadFile) CheckArgs(args map[string]any) llm.ToolAction {
	if path, ok := args["file_path"].(string); ok && isSensitivePath(path) {
		return llm.ToolActionAsk
	}
	return llm.ToolActionAllow
}

// Execute reads the file at args["file_path"] if it is under the tool's root.
// File contents are returned as UTF-8 text; invalid UTF-8 in the file is passed through as-is.
func (r *ReadFile) Execute(ctx context.Context, args map[string]any) (string, error) {
	filePath, err := parseRequiredString(args, "file_path")
	if err != nil {
		return "", err
	}

	resolved, err := r.resolveFilePath(filePath)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", normalizeOSError(err)
	}

	offset, limit := parseOffsetLimit(args)
	return extractLineRange(data, offset, limit)
}

// parseOffsetLimit reads offset and limit from args. Defaults: offset=1, limit=DefaultLimit.
// Values < 1 are clamped to the default.
func parseOffsetLimit(args map[string]any) (offset, limit int) {
	offset = parseOptionalInt(args, "offset", 1)
	if offset < 1 {
		offset = 1
	}
	limit = parseOptionalInt(args, "limit", DefaultLimit)
	if limit < 1 {
		limit = DefaultLimit
	}
	return offset, limit
}

// extractLineRange returns lines [offset, offset+limit) (1-based offset), joined by newline.
// If the range is past the end of the file, returns available lines and a trailing note if there were more lines.
func extractLineRange(data []byte, offset, limit int) (string, error) {
	lines := strings.Split(string(data), "\n")
	total := len(lines)
	// offset is 1-based; start index is offset-1
	start := offset - 1
	if start < 0 {
		start = 0
	}
	if start >= total {
		return "(file has " + strconv.Itoa(total) + " line(s); requested offset is past end)", nil
	}
	end := start + limit
	if end > total {
		end = total
	}
	chunk := lines[start:end]
	out := strings.Join(chunk, "\n")
	if end < total {
		out += "\n\n(showing lines " + strconv.Itoa(start+1) + "-" + strconv.Itoa(end) + " of " + strconv.Itoa(total) + "; use offset/limit to read more)"
	}
	return out, nil
}
