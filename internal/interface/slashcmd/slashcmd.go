// Package slashcmd is the single source of truth for the runtime chat's
// slash-command set. Both the terminal TUI (internal/interface/cli) and the
// Desktop app (internal/interface/desktop, over a Wails binding) read this
// list, so the two surfaces no longer maintain parallel command tables that
// drift apart. It describes the commands; each surface still owns how it
// dispatches and renders them.
package slashcmd

import "slices"

// Surface is a client that presents commands. A command can be offered on some
// surfaces and not others: /sessions has no place in Desktop, where the session
// list is always visible in the sidebar.
type Surface uint8

const (
	// CLI is the terminal TUI.
	CLI Surface = 1 << iota
	// Desktop is the Wails desktop app.
	Desktop
)

// Command describes one slash command. It carries only what both surfaces need
// to present and gate it; the handler that runs it lives in each surface.
type Command struct {
	// Name is the command without its leading slash, e.g. "info".
	Name string `json:"name"`
	// Description is the one-line summary shown in completion and the palette.
	Description string `json:"description"`
	// Surfaces is the set of clients that offer this command.
	Surfaces Surface `json:"-"`
	// RequiresSession marks a command that acts on a saved session, so a surface
	// disables it until the current chat has one (rewind, fork, compact, info).
	RequiresSession bool `json:"requires_session"`
}

// Slash returns the command as typed, with its leading slash.
func (c Command) Slash() string { return "/" + c.Name }

// On reports whether the command is offered on the given surface.
func (c Command) On(s Surface) bool { return c.Surfaces&s != 0 }

// builtins is the ordered command set. Keep it sorted by name: the TUI shows it
// as a completion list and the Desktop palette renders it in order. Add a new
// command here and both surfaces pick it up; each then wires a handler.
var builtins = []Command{
	{Name: "agents", Description: "List available agent types", Surfaces: CLI | Desktop},
	{Name: "compact", Description: "Summarize the conversation so far and continue from the summary", Surfaces: CLI | Desktop, RequiresSession: true},
	{Name: "diff", Description: "Show the working-tree diff for the workspace", Surfaces: CLI | Desktop},
	{Name: "fork", Description: "Branch a new session off an earlier message", Surfaces: CLI | Desktop, RequiresSession: true},
	{Name: "info", Description: "This session's spend and context, and what this project remembers", Surfaces: CLI | Desktop, RequiresSession: true},
	{Name: "mcp", Description: "List connected MCP servers and their status", Surfaces: CLI | Desktop},
	{Name: "model", Description: "Switch the active model, or open the model picker", Surfaces: CLI | Desktop},
	{Name: "plugins", Description: "List installed plugins", Surfaces: CLI | Desktop},
	{Name: "rewind", Description: "Take one of your prompts back to edit and send again", Surfaces: CLI | Desktop, RequiresSession: true},
	{Name: "sessions", Description: "Open the session picker", Surfaces: CLI},
	{Name: "skills", Description: "List the discovered skills", Surfaces: CLI | Desktop},
	{Name: "tasks", Description: "List background jobs", Surfaces: CLI | Desktop},
	{Name: "tools", Description: "List the tools available this run", Surfaces: CLI | Desktop},
	{Name: "worktree", Description: "List this repository's worktrees", Surfaces: CLI | Desktop},
}

// For returns the commands offered on the given surface, in order.
func For(s Surface) []Command {
	out := make([]Command, 0, len(builtins))
	for _, c := range builtins {
		if c.On(s) {
			out = append(out, c)
		}
	}
	return out
}

// Names returns the "/"-prefixed command names offered on the given surface, in
// order. The TUI uses it as its completion list.
func Names(s Surface) []string {
	var out []string
	for _, c := range builtins {
		if c.On(s) {
			out = append(out, c.Slash())
		}
	}
	return out
}

// IsCommand reports whether name (with or without a leading slash) is a builtin
// command on the given surface.
func IsCommand(s Surface, name string) bool {
	name, _ = trimSlash(name)
	return slices.ContainsFunc(builtins, func(c Command) bool {
		return c.On(s) && c.Name == name
	})
}

func trimSlash(s string) (string, bool) {
	if len(s) > 0 && s[0] == '/' {
		return s[1:], true
	}
	return s, false
}
