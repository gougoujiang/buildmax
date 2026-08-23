package agentapp

import (
	"context"
	"log/slog"
	"regexp"
	"sync"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
	"github.com/gougoujiang/buildmax/internal/infra/hook"
)

// HookManager owns the merged hooks configuration, the per-type driver
// registry, and the matcher cache. It is the single object the agent runtime
// interacts with (via agent.HookRunner). Driver polymorphism is invisible
// above this layer.
//
// Concurrency: HookManager is safe to call from multiple goroutines. The
// matcher cache uses a mutex; per-call execution is otherwise stateless.
type HookManager struct {
	cfg     corehook.Config
	drivers map[string]hook.Driver

	mu       sync.Mutex
	matchers map[string]*regexp.Regexp
}

// HookStatus describes the visible state of the manager — counts per event
// and which transport types are configured. Suitable for a future
// `buildmax hooks` inspector or desktop activity view.
type HookStatus struct {
	EventCounts map[string]int `json:"event_counts"`
	Types       []string       `json:"types"`
	TotalHooks  int            `json:"total_hooks"`
}

// NewHookManager constructs a manager from the already-merged hooks config
// and a driver registry. A nil registry is treated as empty; entries whose
// resolved type has no driver are skipped with a warning at dispatch time
// (logged once per event invocation).
func NewHookManager(cfg corehook.Config, drivers map[string]hook.Driver) *HookManager {
	if drivers == nil {
		drivers = map[string]hook.Driver{}
	}
	return &HookManager{
		cfg:      cfg,
		drivers:  drivers,
		matchers: make(map[string]*regexp.Regexp),
	}
}

// Run implements agent.HookRunner. See docs/design/hook-system.md §8.2 for
// the dispatch flow. The first matching entry that returns a block decision
// wins for the gate; every other matching entry still executes so audit
// hooks see every event.
func (m *HookManager) Run(ctx context.Context, in agent.HookInput) agent.HookOutput {
	if m == nil {
		return agent.HookOutput{}
	}
	entries := m.snapshot().Entries(string(in.Event))
	if len(entries) == 0 {
		return agent.HookOutput{}
	}
	var decision agent.HookOutput
	for _, entry := range entries {
		if !m.matches(entry.Matcher, in.ToolName) {
			continue
		}
		driverType := entry.ResolvedType()
		driver, ok := m.drivers[driverType]
		if !ok {
			hookLog().Warn("no driver for type; skipping entry",
				"event", in.Event,
				"type", driverType,
			)
			continue
		}
		out := driver.Run(ctx, entry, in)
		if !decision.Blocked() && out.Blocked() {
			decision = out
		}
	}
	return decision
}

// Refresh swaps the merged config without rebuilding driver instances. The
// matcher cache is preserved so previously compiled regexes are still hot.
// Drivers that watch their own dependencies (HTTP transport, MCP catalog)
// pick up changes via their Deps.
func (m *HookManager) Refresh(cfg corehook.Config) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

// snapshot returns the config to dispatch against. Refresh can swap it while
// a run is in flight, and tool calls now reach Run from several goroutines at
// once, so the read is taken under the lock and the rest of the call works
// from that copy rather than re-reading a field mid-dispatch.
func (m *HookManager) snapshot() corehook.Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// Status returns a snapshot describing what the manager currently dispatches.
func (m *HookManager) Status() HookStatus {
	if m == nil {
		return HookStatus{EventCounts: map[string]int{}}
	}
	events := []string{
		corehook.EventSessionStart,
		corehook.EventSessionEnd,
		corehook.EventUserPromptSubmit,
		corehook.EventPreToolUse,
		corehook.EventPostToolUse,
		corehook.EventPostToolUseFailure,
		corehook.EventNotification,
		corehook.EventPreCompact,
		corehook.EventPostCompact,
		corehook.EventSubagentStart,
		corehook.EventSubagentStop,
		corehook.EventStop,
		corehook.EventStopFailure,
	}
	cfg := m.snapshot()
	counts := make(map[string]int, len(events))
	total := 0
	for _, e := range events {
		n := len(cfg.Entries(e))
		if n > 0 {
			counts[e] = n
			total += n
		}
	}
	types := make([]string, 0, len(m.drivers))
	for t := range m.drivers {
		types = append(types, t)
	}
	return HookStatus{EventCounts: counts, Types: types, TotalHooks: total}
}

// matches reports whether pattern applies to toolName. An empty pattern
// matches every invocation. Non-tool events pass an empty toolName, so a
// non-empty pattern always rejects them — tool-name matchers do not apply
// to lifecycle events.
func (m *HookManager) matches(pattern, toolName string) bool {
	if pattern == "" {
		return true
	}
	if toolName == "" {
		return false
	}
	re := m.compile(pattern)
	if re == nil {
		return false
	}
	return re.MatchString(toolName)
}

func (m *HookManager) compile(pattern string) *regexp.Regexp {
	m.mu.Lock()
	defer m.mu.Unlock()
	if re, ok := m.matchers[pattern]; ok {
		return re
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		hookLog().Warn("invalid matcher regex; entry will never match", "pattern", pattern, "err", err)
		m.matchers[pattern] = nil
		return nil
	}
	m.matchers[pattern] = re
	return re
}

// Identity belongs in an attr, not in every message string.
func hookLog() *slog.Logger { return slog.With("component", "hook") }
