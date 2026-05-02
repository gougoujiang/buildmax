// Package model holds shared contracts used across core services and agent execution.
// Keep this package at the bottom of the dependency graph to avoid import cycles.
package model

import "context"

// Tool is a capability the agent can invoke by name.
// Both the core agent loop (internal/core/agent) and runtime tools (internal/execution/agenttool) use this interface.
type Tool interface {
	Name() string
	Description() string
	Parameters() any // JSON schema for arguments (e.g. map[string]any)
	Execute(ctx context.Context, args map[string]any) (result string, err error)
}
