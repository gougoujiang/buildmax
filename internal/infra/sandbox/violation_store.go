package sandbox

import (
	"strings"
	"sync"
	"time"
)

// Violation is one recorded sandbox decision (allow or deny) for status
// display, trace export, and audit hooks. The schema mirrors
// docs/design/sandbox-boundaries.md §11.3.
type Violation struct {
	Time       time.Time `json:"time"`
	Kind       string    `json:"kind"`           // "net_deny" | "net_allow" | "sandbox_disabled" | "backend_unavailable"
	Tool       string    `json:"tool,omitempty"` // e.g. "Bash", "WebFetch"
	Host       string    `json:"host,omitempty"` // network events
	Reason     string    `json:"reason,omitempty"`
	Suppressed bool      `json:"suppressed,omitempty"` // hidden from display via ignore_violations
}

// Violation kinds.
const (
	ViolationNetAllow           = "net_allow"
	ViolationNetDeny            = "net_deny"
	ViolationSandboxDisabled    = "sandbox_disabled"
	ViolationBackendUnavailable = "backend_unavailable"
)

// ViolationStore is a bounded ring buffer of Violations. Cheap to keep
// around for the life of the Manager; size is small (default 256) so
// memory is negligible. Concurrent-safe.
type ViolationStore struct {
	mu      sync.Mutex
	entries []Violation
	max     int
	idx     int // next write slot
	totals  map[string]uint64
}

// DefaultViolationCapacity bounds the store. Newer entries displace older.
const DefaultViolationCapacity = 256

// NewViolationStore returns an empty store with the given capacity. A
// non-positive capacity falls back to DefaultViolationCapacity.
func NewViolationStore(capacity int) *ViolationStore {
	if capacity <= 0 {
		capacity = DefaultViolationCapacity
	}
	return &ViolationStore{
		max:    capacity,
		totals: make(map[string]uint64, 8),
	}
}

// Add records v. Newest entries replace the oldest once capacity is hit.
// Returns the stored Violation (with Suppressed possibly toggled by an
// upstream filter; this store does not filter on its own).
func (s *ViolationStore) Add(v Violation) {
	if s == nil {
		return
	}
	if v.Time.IsZero() {
		v.Time = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) < s.max {
		s.entries = append(s.entries, v)
	} else {
		s.entries[s.idx] = v
		s.idx = (s.idx + 1) % s.max
	}
	s.totals[v.Kind]++
}

// Recent returns up to n most-recent entries in chronological order
// (oldest→newest). n<=0 returns every entry the store still holds.
func (s *ViolationStore) Recent(n int) []Violation {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) == 0 {
		return nil
	}
	// Build a stable chronological view by walking from the oldest slot.
	out := make([]Violation, 0, len(s.entries))
	if len(s.entries) < s.max {
		out = append(out, s.entries...)
	} else {
		out = append(out, s.entries[s.idx:]...)
		out = append(out, s.entries[:s.idx]...)
	}
	if n > 0 && len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// Totals returns cumulative counts per Kind since the store was created.
func (s *ViolationStore) Totals() map[string]uint64 {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]uint64, len(s.totals))
	for k, v := range s.totals {
		out[k] = v
	}
	return out
}

// IgnoreFilter wraps a Violator and tags entries that match an
// `ignore_violations` rule from settings.yaml. Suppressed entries still
// reach the store so totals stay correct; the display layer hides them.
//
// Each rule is matched against `<kind>:<host-or-reason>` so users can
// suppress noisy patterns like "net_deny:metrics.example.com" while
// still seeing the broader decision stream.
type IgnoreFilter struct {
	store    *ViolationStore
	rulesFor func(tool string) []string
}

// NewIgnoreFilter constructs a filter that consults rulesFor(toolName)
// for each violation; nil or empty result skips suppression for that tool.
func NewIgnoreFilter(store *ViolationStore, rulesFor func(tool string) []string) *IgnoreFilter {
	return &IgnoreFilter{store: store, rulesFor: rulesFor}
}

// Record decides whether to mark v as suppressed before adding to store.
func (f *IgnoreFilter) Record(v Violation) {
	if f == nil || f.store == nil {
		return
	}
	if f.rulesFor != nil {
		patterns := f.rulesFor(v.Tool)
		if len(patterns) > 0 {
			key := v.Kind
			if v.Host != "" {
				key += ":" + v.Host
			} else if v.Reason != "" {
				key += ":" + v.Reason
			}
			for _, p := range patterns {
				if matchIgnorePattern(p, key, v) {
					v.Suppressed = true
					break
				}
			}
		}
	}
	f.store.Add(v)
}

// matchIgnorePattern reports whether pattern suppresses key. Patterns
// are matched literally against `<kind>` or `<kind>:<host>` (case-
// insensitive), with a trailing `*` as a prefix wildcard.
func matchIgnorePattern(pattern, key string, _ Violation) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	pat := strings.ToLower(pattern)
	low := strings.ToLower(key)
	if prefix, ok := strings.CutSuffix(pat, "*"); ok {
		return strings.HasPrefix(low, prefix)
	}
	return pat == low
}

// proxyViolator adapts an IgnoreFilter to the Proxy's Violator interface.
// One per Manager; the proxy hands every allow/deny decision through here.
type proxyViolator struct {
	filter *IgnoreFilter
}

func (p *proxyViolator) OnAllow(host string) {
	if p == nil || p.filter == nil {
		return
	}
	p.filter.Record(Violation{Kind: ViolationNetAllow, Tool: "Bash", Host: host})
}

func (p *proxyViolator) OnDeny(host, reason string) {
	if p == nil || p.filter == nil {
		return
	}
	p.filter.Record(Violation{Kind: ViolationNetDeny, Tool: "Bash", Host: host, Reason: reason})
}
