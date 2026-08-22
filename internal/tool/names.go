// Package tool provides concrete agent tools (Read, Write, Edit, Glob, Grep, Bash,
// WebFetch, TodoWrite, NoteWrite, Skill, Task, and MCP gateway tools).
package tool

// Tool name constants — single source of truth for every tool's Name(). Use camelCase for LLM-facing names.
const (
	ToolNameRead      = "Read"
	ToolNameWrite     = "Write"
	ToolNameEdit      = "Edit"
	ToolNameGlob      = "Glob"
	ToolNameGrep      = "Grep"
	ToolNameBash      = "Bash"
	ToolNameWebFetch  = "WebFetch"
	ToolNameTodoWrite = "TodoWrite"
	ToolNameNoteWrite = "NoteWrite"
	ToolNameSkill     = "Skill"
	ToolNameTask      = "Task"
	// ToolNameUploadArtifact is registered only where the surface has an
	// artifact service; a session with none does not offer it at all.
	ToolNameUploadArtifact = "UploadArtifact"
	// The Job tools and Monitor are registered only where local background
	// jobs are enabled (TUI and Desktop), and never inside subagents.
	ToolNameJobList   = "JobList"
	ToolNameJobOutput = "JobOutput"
	ToolNameJobStop   = "JobStop"
	ToolNameMonitor   = "Monitor"
)
