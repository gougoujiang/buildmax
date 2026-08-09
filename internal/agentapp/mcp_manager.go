package agentapp

import (
	"context"
	"fmt"
	"sync"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/infra/mcp"
)

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
func NewMCPManager(ctx context.Context, cfg *config.MCPConfigRoot) (*MCPManager, error) {
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
func (m *MCPManager) Refresh(ctx context.Context, cfg *config.MCPConfigRoot) error {
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
