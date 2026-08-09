// Package tool provides concrete agent tools (Read, Write, Edit, Glob, Grep, Bash,
// WebFetch, TodoWrite, Skill, Task, and MCP gateway tools).
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
	ToolNameSkill     = "Skill"
	ToolNameTask      = "Task"
)
