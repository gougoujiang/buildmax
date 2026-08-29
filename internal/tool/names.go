// Package tool provides concrete agent tools. Every runtime gets Read, Write,
// Edit, Glob, Grep, Bash, WebFetch, TodoWrite, NoteWrite, Skill, Task, and the
// MCP gateway tools; UploadArtifact, Worktree, GetIssue, ReportToIssue,
// JobList, JobOutput, JobStop, and Monitor are registered only where the
// surface can serve them, as the constants below say.
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
	// The Issue tools are registered only where the run is working one Issue
	// and can reach it -- a worker run started from an Issue, or a logged-in
	// local session linked to one -- and never inside subagents, which report
	// to their parent rather than to a team's thread. Both are scoped to that
	// one Issue when they are built; neither takes an issue id. See
	// docs/design/issue-agent-access.md.
	ToolNameGetIssue      = "GetIssue"
	ToolNameReportToIssue = "ReportToIssue"
	// The Job tools and Monitor are registered only where local background
	// jobs are enabled (TUI and Desktop), and never inside subagents.
	ToolNameJobList   = "JobList"
	ToolNameJobOutput = "JobOutput"
	ToolNameJobStop   = "JobStop"
	ToolNameMonitor   = "Monitor"
)
