// Package tool provides concrete agent tools. Every runtime gets Read, Write,
// Edit, Glob, Grep, Bash, WebFetch, TodoWrite, NoteWrite, Skill, Task, and the
// MCP gateway tools; UploadArtifact, Worktree, JobList, JobOutput, JobStop,
// and Monitor are registered only where the surface can serve them, as the
// constants below say.
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
	// ToolNameWorktree is registered only where a session may move its own
	// workspace root — the CLI and TUI — and never inside subagents, which
	// share the parent's root for the length of their run.
	ToolNameWorktree = "Worktree"
	// ToolNameProjectMemoryWrite is registered only on a local primary run
	// whose session belongs to a Project and whose user has not turned memory
	// off. A subagent reads the same memory but does not get this: the parent
	// stays the single curator for one task. See
	// docs/design/local-project-memory.md §9.4.
	ToolNameProjectMemoryWrite = "ProjectMemoryWrite"
	// The Job tools and Monitor are registered only where local background
	// jobs are enabled (TUI and Desktop), and never inside subagents.
	ToolNameJobList   = "JobList"
	ToolNameJobOutput = "JobOutput"
	ToolNameJobStop   = "JobStop"
	ToolNameMonitor   = "Monitor"
)
