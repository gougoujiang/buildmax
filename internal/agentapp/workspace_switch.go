package agentapp

import (
	"context"
	"log/slog"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// derivedResolveTimeout bounds the work a root move does. Re-resolving means
// restarting MCP servers, and a server that will not come up must not hold the
// session in the tree it is moving out of.
const derivedResolveTimeout = 30 * time.Second

// sessionRoot is what the worktree manager moves.
//
// Set does the move and re-resolves everything derived from the root in one
// place, because the two cannot be allowed to drift: a session that moved its
// root but kept the launch tree's hooks, skills, and MCP servers would be
// running one tree's configuration against another tree's files, and nothing
// about it would look wrong. See docs/design/workspace-root-and-worktrees.md §4.
type sessionRoot struct{ app *AgentApp }

func (s sessionRoot) Root() string { return s.app.workspace.Root() }

func (s sessionRoot) Set(dir string) {
	s.app.workspace.Set(dir)
	s.app.resolveDerivedFromRoot()
}

// resolveDerivedFromRoot reloads the configuration the workspace root decides:
// hooks, skills, subagent definitions, and MCP servers.
//
// Failures are logged rather than returned. The move has already happened by
// the time this runs, and the alternatives are both worse than a degraded
// layer: refusing the move would strand the session between two trees, and
// unwinding it would undo work the user asked for. Hooks failing open is the
// standing rule in docs/design/hook-system.md; MCP trouble stays visible in
// `/mcp`.
//
// The AGENTS.md prompt layer is deliberately absent: RunLoop builds the prompt
// from the current root on every turn, so the layer already follows a move
// without anything here re-reading it. TestPromptLayerFollowsTheRoot holds
// that property.
func (a *AgentApp) resolveDerivedFromRoot() {
	if a == nil {
		return
	}
	root := a.workspace.Root()

	if workspaceHooks, err := config.LoadWorkspaceHooks(root); err != nil {
		slog.Warn("worktree switch: workspace hooks not reloaded", "root", root, "err", err)
	} else {
		a.hookManager.Refresh(config.MergeHooks(a.settings.Hooks, a.pluginHooks, workspaceHooks))
	}

	plugins := a.plugins.Loadable()
	if err := a.skillsRegistry.Load(root, plugins); err != nil {
		slog.Warn("worktree switch: skills not reloaded", "root", root, "err", err)
	}
	if err := a.subagentsRegistry.Load(root, plugins); err != nil {
		slog.Warn("worktree switch: subagent definitions not reloaded", "root", root, "err", err)
	}

	// Before MCP, not after: a refresh that fails part way must still leave the
	// registries dropped, or the session keeps being offered the launch tree's
	// skills because the slow step further down errored.
	a.invalidateToolRegistries()

	// Last: it is the slow one, and a server that will not start should not
	// hold up everything above it.
	if a.mcpManager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), derivedResolveTimeout)
		defer cancel()
		if _, err := a.RefreshMCP(ctx); err != nil {
			slog.Warn("worktree switch: mcp servers not reloaded", "root", root, "err", err)
		}
	}
}

// invalidateToolRegistries drops the per-model tool registry cache.
//
// Tools resolve paths through the workspace and need no rebuild, but the Skill
// tool and the subagent types are built from registry snapshots taken when the
// registry was cached. Left alone, a session in a worktree would be offered the
// launch tree's skills.
func (a *AgentApp) invalidateToolRegistries() {
	a.toolRegistriesMu.Lock()
	defer a.toolRegistriesMu.Unlock()
	a.toolRegistries = make(map[string]cllm.ToolRegistry)
}
