package hook

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// MCPDriver invokes a tool on an already-connected MCP server as a hook.
// The driver delegates the actual MCP call to a MCPCaller (provided by
// agentapp). Values inside entry.Input may reference fields of the
// HookInput payload via "${field}" placeholders (e.g. "${tool_args.path}")
// so the hook script can pass per-event data to the MCP tool.
//
// The tool's text result is parsed as JSON when it looks like a hook output
// object ({decision, reason}); other text is treated as allow.
type MCPDriver struct {
	caller MCPCaller
}

// NewMCPDriver returns a driver backed by caller. A nil caller makes the
// driver fail open with a warning at dispatch time (consistent with other
// missing-driver scenarios).
func NewMCPDriver(caller MCPCaller) *MCPDriver { return &MCPDriver{caller: caller} }

// Type satisfies Driver.
func (MCPDriver) Type() string { return config.HookTypeMCP }

// Run invokes the configured MCP tool.
func (d *MCPDriver) Run(ctx context.Context, entry config.HookEntry, in agent.HookInput) agent.HookOutput {
	if d == nil || d.caller == nil {
		slog.Warn("hook: mcp_tool driver has no caller; failing open", "event", in.Event)
		return agent.HookOutput{}
	}
	if entry.Server == "" || entry.Tool == "" {
		slog.Warn("hook: mcp_tool entry missing server or tool",
			"event", in.Event, "server", entry.Server, "tool", entry.Tool)
		return agent.HookOutput{}
	}

	mcpInput, err := materializeMCPInput(entry.Input, in)
	if err != nil {
		slog.Warn("hook: build mcp input failed; failing open", "event", in.Event, "err", err)
		return agent.HookOutput{}
	}

	callCtx, cancel := context.WithTimeout(ctx, resolveTimeout(entry.Timeout))
	defer cancel()

	result, err := d.caller.CallMCPTool(callCtx, entry.Server, entry.Tool, mcpInput)
	if err != nil {
		slog.Warn("hook: mcp call failed; failing open",
			"event", in.Event,
			"server", entry.Server,
			"tool", entry.Tool,
			"err", err,
		)
		return agent.HookOutput{}
	}

	out, ok := parseHookOutput([]byte(result))
	if !ok {
		slog.Debug("hook: mcp tool returned no decision", "event", in.Event, "server", entry.Server, "tool", entry.Tool)
		return agent.HookOutput{}
	}
	if out.Blocked() {
		slog.Info("hook: mcp tool blocked", "event", in.Event, "tool", in.ToolName, "server", entry.Server, "mcp_tool", entry.Tool, "reason", out.Reason)
	}
	return out
}

// materializeMCPInput substitutes "${field}" / "${field.sub}" references in
// entry.Input against the HookInput payload. Unknown fields expand to the
// empty string (consistent with HTTP header expansion). Other values pass
// through unchanged.
func materializeMCPInput(template map[string]any, in agent.HookInput) (map[string]any, error) {
	if len(template) == 0 {
		return nil, nil
	}
	// Build a JSON-shaped view of the HookInput so dotted references like
	// "tool_args.path" can be resolved generically.
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var view map[string]any
	if err := json.Unmarshal(raw, &view); err != nil {
		return nil, err
	}
	out := make(map[string]any, len(template))
	for k, v := range template {
		out[k] = expandHookPayloadValue(v, view)
	}
	return out, nil
}

// expandHookPayloadValue walks a YAML-decoded value, expanding ${ref} inside
// strings and recursing into maps / slices. Non-string scalars pass through.
func expandHookPayloadValue(v any, view map[string]any) any {
	switch x := v.(type) {
	case string:
		return expandHookRef(x, view)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = expandHookPayloadValue(vv, view)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = expandHookPayloadValue(vv, view)
		}
		return out
	default:
		return v
	}
}

// expandHookRef replaces every "${field}" or "${field.sub}" in s with the
// corresponding value from view. Missing references expand to "".
// Non-templated strings return unchanged.
func expandHookRef(s string, view map[string]any) string {
	if !strings.Contains(s, "${") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '$' && s[i+1] == '{' {
			end := strings.Index(s[i:], "}")
			if end == -1 {
				b.WriteString(s[i:])
				break
			}
			ref := s[i+2 : i+end]
			b.WriteString(resolveDottedRef(ref, view))
			i += end + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func resolveDottedRef(ref string, view map[string]any) string {
	parts := strings.Split(ref, ".")
	var cur any = view
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		v, exists := m[p]
		if !exists {
			return ""
		}
		cur = v
	}
	switch v := cur.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}
