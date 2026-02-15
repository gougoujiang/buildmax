# Util

## Purpose

The `internal/util` package provides small helpers used across the codebase: workspace path resolution (for tools), Git branch detection, and argument parsing. It has no single theme beyond "shared utilities."

## Key Types and Functions

| Name | Kind | Role |
|------|------|------|
| **Workspace** | struct | Holds an absolute root path; created with `NewWorkspace(root)` |
| **ResolvePath** | method | Resolves a user path relative to the workspace root; ensures result stays under root (safe for tools) |
| **CurrentBranch** | func | Runs `git branch --show-current` in a directory; returns branch name or empty string |
| **argparse** | (pkg) | Argument parsing helpers used by tools or CLI |

## How It Works

- **Workspace**: Used by the agent and tools to scope file operations. Empty root means current working directory. `ResolvePath` is Windows-safe and does not stat the path.
- **Git**: `CurrentBranch(dir)` returns the current branch when `dir` is a Git repo root; otherwise returns `""`.

## Dependencies

- **Used by**: `internal/tools` (workspace for read/write/edit/glob etc.), other callers that need path or git helpers

## Notes

- See [Tools](tools.md) for how workspace is passed into tool execution.
