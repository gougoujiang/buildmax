package agentapp

import (
	"context"
	"fmt"
	"sort"
	"sync"

	mcpcfg "github.com/icloudbb/buildmax/internal/core/mcp"
	"github.com/icloudbb/buildmax/internal/infra/mcp"
)

// rejectWorkerStdioMCP fails an unattended-worker run whose resolved MCP
// configuration declares a stdio server.
//
// The worker launches a stdio server as a direct child process, outside the
// Bash boundary the worker profile confines model-chosen commands to, so the
// supported profile disables the transport rather than run it unconfined. This
// runs against the fully merged configuration — global, workspace, and
// activated-plugin layers — before NewMCPManager starts any child, so the
// refusal lands before the command executes and before the first model call.
//
// The message names the rejected server ids, sorted for a stable diagnostic,
// and the remote transports that are supported instead. It never prints the
// command, arguments, environment, or url, which can carry secrets.
func rejectWorkerStdioMCP(cfg *mcpcfg.ConfigRoot) error {
	if cfg == nil {
		return nil
	}
	var stdio []string
	for id, s := range cfg.MCPServers {
		if s.Type == mcpcfg.TransportStdio {
			stdio = append(stdio, id)
		}
	}
	if len(stdio) == 0 {
		return nil
	}
	sort.Strings(stdio)
	return fmt.Errorf("the unattended worker profile does not support stdio MCP servers %v: "+
		"the worker would run them as unconfined child processes outside its Bash boundary; "+
		"configure a remote transport (%s or %s) instead",
		stdio, mcpcfg.TransportHTTP, mcpcfg.TransportSSE)
}

// resolvedRemoteTransports returns the sorted, deduplicated remote transport
// kinds (a subset of "http","sse") the resolved config declares.
//
// It names kinds only, never a server id, command, url, or credential, because
// its one consumer is the trace's MCP treatment, which an operator reads.
func resolvedRemoteTransports(cfg *mcpcfg.ConfigRoot) []string {
	if cfg == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, s := range cfg.MCPServers {
		switch s.Type {
		case mcpcfg.TransportHTTP, mcpcfg.TransportSSE:
			seen[s.Type] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type MCPStatus struct {
	LoadError string
	Servers   []mcp.MCPServerStatus
}

type MCPManager struct {
	mu       sync.Mutex
	registry *mcp.Registry
	status   MCPStatus
}

// NewMCPManager performs an initial Refresh with the provided config. Config load
// failures are returned as errors; individual server connection failures are
// surfaced only via Status().
func NewMCPManager(ctx context.Context, cfg *mcpcfg.ConfigRoot) (*MCPManager, error) {
	mgr := &MCPManager{}
	if err := mgr.Refresh(ctx, cfg); err != nil {
		return nil, err
	}
	return mgr, nil
}

func (m *MCPManager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	reg := m.registry
	m.registry = nil
	m.mu.Unlock()
	if reg == nil {
		return nil
	}
	return reg.Close()
}

func (m *MCPManager) Registry() *mcp.Registry {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registry
}

func (m *MCPManager) Status() MCPStatus {
	if m == nil {
		return MCPStatus{LoadError: "mcp manager is not initialized"}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	servers := make([]mcp.MCPServerStatus, len(m.status.Servers))
	copy(servers, m.status.Servers)
	return MCPStatus{
		LoadError: m.status.LoadError,
		Servers:   servers,
	}
}

// Refresh reconnects to all servers in cfg. Individual server connection failures
// are non-fatal and are recorded in Status() instead.
func (m *MCPManager) Refresh(ctx context.Context, cfg *mcpcfg.ConfigRoot) error {
	if m == nil {
		return fmt.Errorf("mcp manager is not initialized")
	}

	if cfg == nil || len(cfg.MCPServers) == 0 {
		m.swapState(nil, MCPStatus{})
		return nil
	}

	reg, rows := mcp.LoadRegistryState(ctx, cfg, nil)
	status := MCPStatus{Servers: rows}
	if reg == nil {
		for _, row := range rows {
			if row.Err != nil {
				status.LoadError = row.Err.Error()
				break
			}
		}
	}
	m.swapState(reg, status)
	return nil
}

func (m *MCPManager) swapState(next *mcp.Registry, status MCPStatus) {
	m.mu.Lock()
	prev := m.registry
	m.registry = next
	m.status = status
	m.mu.Unlock()
	if prev != nil && prev != next {
		_ = prev.Close()
	}
}
