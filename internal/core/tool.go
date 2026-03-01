// Package core holds shared types and interfaces used by the agent runtime.
// Put here only types that are used by two or more of: agent, tools, conversation.
// This package does not depend on agent, tools, or conversation, so it stays at the
// bottom of the dependency graph and avoids import cycles.
package core

import "context"

// Tool is a capability the agent can invoke by name.
// Both the CLI agent (internal/agent) and tools (internal/tools) use this interface.
type Tool interface {
	Name() string
	Description() string
	Parameters() any // JSON schema for arguments (e.g. map[string]any)
	Execute(ctx context.Context, args map[string]any) (result string, err error)
}
