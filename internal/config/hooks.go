package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// WorkspaceHooksPath returns the per-workspace hooks config path
// (<workspace>/.buildmax/hooks.yaml). It does not check existence.
func WorkspaceHooksPath(workspace string) string {
	return filepath.Join(workspace, ".buildmax", "hooks.yaml")
}

// LoadWorkspaceHooks reads <workspace>/.buildmax/hooks.yaml. A missing file
// is not an error — returns (HooksConfig{}, nil) so callers can merge
// unconditionally. A malformed file is reported as an error so misconfig
// surfaces at startup instead of silently dropping rules.
//
// The file shape mirrors the "hooks:" block of settings.yaml; the top-level
// keys are pre_tool_use / post_tool_use / pre_compact / post_compact /
// run_end (the same snake_case event keys).
func LoadWorkspaceHooks(workspace string) (HooksConfig, error) {
	if workspace == "" {
		return HooksConfig{}, nil
	}
	return loadHooksFile(WorkspaceHooksPath(workspace))
}

// loadHooksFile reads one hooks.yaml. A missing file is not an error; a
// malformed one is, so misconfiguration surfaces instead of silently dropping
// rules.
func loadHooksFile(path string) (HooksConfig, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return HooksConfig{}, nil
		}
		return HooksConfig{}, fmt.Errorf("stat hooks %q: %w", path, err)
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return HooksConfig{}, fmt.Errorf("read hooks %q: %w", path, err)
	}
	var cfg HooksConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return HooksConfig{}, fmt.Errorf("parse hooks %q: %w", path, err)
	}
	return cfg, nil
}

// Event keys recognised in settings.yaml under "hooks". The values match
// agent.HookEvent so callers can pass them through directly.
const (
	HookEventSessionStart       = "SessionStart"
	HookEventSessionEnd         = "SessionEnd"
	HookEventUserPromptSubmit   = "UserPromptSubmit"
	HookEventPreToolUse         = "PreToolUse"
	HookEventPostToolUse        = "PostToolUse"
	HookEventPostToolUseFailure = "PostToolUseFailure"
	HookEventNotification       = "Notification"
	HookEventPreCompact         = "PreCompact"
	HookEventPostCompact        = "PostCompact"
	HookEventSubagentStart      = "SubagentStart"
	HookEventSubagentStop       = "SubagentStop"
	HookEventStop               = "Stop"
	HookEventStopFailure        = "StopFailure"
)

// DefaultHookTimeoutSecs is the per-hook execution timeout when a hook entry
// does not set its own timeout. Long enough for formatters/linters; short
// enough that a hung script does not block the agent for a full LLM turn.
const DefaultHookTimeoutSecs = 30

// Hook transport types. Match the canonical names used by Claude Code so
// scripts and operator docs port directly.
const (
	HookTypeCommand = "command"  // shell command on stdin/stdout
	HookTypeHTTP    = "http"     // HTTP POST with JSON body
	HookTypeMCP     = "mcp_tool" // MCP tool invocation
	HookTypePrompt  = "prompt"   // single-turn LLM prompt
)

// DefaultHookType is the transport assumed when a HookEntry omits Type.
// "command" preserves back-compat with pre-v2 settings.yaml files.
const DefaultHookType = HookTypeCommand

// HookEntry is one configured hook invocation. The Type discriminator
// selects which transport-specific fields are read; entries with no Type
// default to HookTypeCommand for back-compat with pre-v2 settings.
//
// Matcher is a regular expression evaluated against the tool name for
// PreToolUse / PostToolUse; an empty Matcher matches every invocation. For
// non-tool events the field is ignored.
//
// Timeout is per-hook in seconds. Zero means DefaultHookTimeoutSecs.
//
// Per-type fields are kept on one flat struct so a HookEntry round-trips
// cleanly through YAML/JSON without a custom unmarshaller. A driver only
// reads the fields relevant to its Type; the rest are zero values.
type HookEntry struct {
	// Type selects the transport: "command", "http", "mcp_tool", "prompt".
	// Empty defaults to HookTypeCommand.
	Type    string `mapstructure:"type"     json:"type,omitempty"     yaml:"type,omitempty"`
	Matcher string `mapstructure:"matcher"  json:"matcher,omitempty"  yaml:"matcher,omitempty"`
	Timeout int    `mapstructure:"timeout"  json:"timeout,omitempty"  yaml:"timeout,omitempty"`

	// Command transport: shell command to execute via "sh -c" (Unix) or
	// "cmd /C" (Windows). Stdin receives the HookInput JSON.
	Command string `mapstructure:"command"  json:"command,omitempty"  yaml:"command,omitempty"`

	// HTTP transport: POSTs the HookInput JSON to URL. Headers values may
	// contain "$VAR" / "${VAR}" references; only env vars whitelisted in
	// AllowedEnv may be interpolated.
	URL        string            `mapstructure:"url"          json:"url,omitempty"          yaml:"url,omitempty"`
	Headers    map[string]string `mapstructure:"headers"      json:"headers,omitempty"      yaml:"headers,omitempty"`
	AllowedEnv []string          `mapstructure:"allowed_env"  json:"allowed_env,omitempty"  yaml:"allowed_env,omitempty"`

	// MCP transport: invoke Tool on Server with Input. Input values may
	// contain "${field}" references resolved against the HookInput payload.
	Server string         `mapstructure:"server"  json:"server,omitempty"  yaml:"server,omitempty"`
	Tool   string         `mapstructure:"tool"    json:"tool,omitempty"    yaml:"tool,omitempty"`
	Input  map[string]any `mapstructure:"input"   json:"input,omitempty"   yaml:"input,omitempty"`

	// Prompt transport: run a single-turn prompt against Model (empty uses
	// the default fast model). The literal "$ARGUMENTS" inside Prompt is
	// replaced with the HookInput JSON.
	Prompt string `mapstructure:"prompt"  json:"prompt,omitempty"  yaml:"prompt,omitempty"`
	Model  string `mapstructure:"model"   json:"model,omitempty"   yaml:"model,omitempty"`
}

// ResolvedType returns Entry.Type, defaulting to HookTypeCommand when empty.
func (e HookEntry) ResolvedType() string {
	if e.Type == "" {
		return DefaultHookType
	}
	return e.Type
}

// HooksConfig is the "hooks" block of settings.yaml. Per CLAUDE.md §6.1 the
// keys are snake_case; the values still resolve to the canonical HookEvent
// names listed above.
type HooksConfig struct {
	SessionStart       []HookEntry `mapstructure:"session_start"          json:"session_start,omitempty"          yaml:"session_start,omitempty"`
	SessionEnd         []HookEntry `mapstructure:"session_end"            json:"session_end,omitempty"            yaml:"session_end,omitempty"`
	UserPromptSubmit   []HookEntry `mapstructure:"user_prompt_submit"     json:"user_prompt_submit,omitempty"     yaml:"user_prompt_submit,omitempty"`
	PreToolUse         []HookEntry `mapstructure:"pre_tool_use"           json:"pre_tool_use,omitempty"           yaml:"pre_tool_use,omitempty"`
	PostToolUse        []HookEntry `mapstructure:"post_tool_use"          json:"post_tool_use,omitempty"          yaml:"post_tool_use,omitempty"`
	PostToolUseFailure []HookEntry `mapstructure:"post_tool_use_failure"  json:"post_tool_use_failure,omitempty"  yaml:"post_tool_use_failure,omitempty"`
	Notification       []HookEntry `mapstructure:"notification"           json:"notification,omitempty"           yaml:"notification,omitempty"`
	PreCompact         []HookEntry `mapstructure:"pre_compact"            json:"pre_compact,omitempty"            yaml:"pre_compact,omitempty"`
	PostCompact        []HookEntry `mapstructure:"post_compact"           json:"post_compact,omitempty"           yaml:"post_compact,omitempty"`
	SubagentStart      []HookEntry `mapstructure:"subagent_start"         json:"subagent_start,omitempty"         yaml:"subagent_start,omitempty"`
	SubagentStop       []HookEntry `mapstructure:"subagent_stop"          json:"subagent_stop,omitempty"          yaml:"subagent_stop,omitempty"`
	Stop               []HookEntry `mapstructure:"stop"                   json:"stop,omitempty"                   yaml:"stop,omitempty"`
	StopFailure        []HookEntry `mapstructure:"stop_failure"           json:"stop_failure,omitempty"           yaml:"stop_failure,omitempty"`
}

// HookEventNames lists every event in dispatch order, for a surface that has to
// enumerate them.
func HookEventNames() []string {
	return []string{
		HookEventSessionStart, HookEventSessionEnd, HookEventUserPromptSubmit,
		HookEventPreToolUse, HookEventPostToolUse, HookEventPostToolUseFailure,
		HookEventNotification, HookEventPreCompact, HookEventPostCompact,
		HookEventSubagentStart, HookEventSubagentStop,
		HookEventStop, HookEventStopFailure,
	}
}

// Entries returns the configured hooks for the named event, in declared order.
// Unknown events return nil.
func (h HooksConfig) Entries(event string) []HookEntry {
	switch event {
	case HookEventSessionStart:
		return h.SessionStart
	case HookEventSessionEnd:
		return h.SessionEnd
	case HookEventUserPromptSubmit:
		return h.UserPromptSubmit
	case HookEventPreToolUse:
		return h.PreToolUse
	case HookEventPostToolUse:
		return h.PostToolUse
	case HookEventPostToolUseFailure:
		return h.PostToolUseFailure
	case HookEventNotification:
		return h.Notification
	case HookEventPreCompact:
		return h.PreCompact
	case HookEventPostCompact:
		return h.PostCompact
	case HookEventSubagentStart:
		return h.SubagentStart
	case HookEventSubagentStop:
		return h.SubagentStop
	case HookEventStop:
		return h.Stop
	case HookEventStopFailure:
		return h.StopFailure
	default:
		return nil
	}
}

// IsEmpty reports whether the configuration has no hooks at all.
func (h HooksConfig) IsEmpty() bool {
	return len(h.SessionStart) == 0 &&
		len(h.SessionEnd) == 0 &&
		len(h.UserPromptSubmit) == 0 &&
		len(h.PreToolUse) == 0 &&
		len(h.PostToolUse) == 0 &&
		len(h.PostToolUseFailure) == 0 &&
		len(h.Notification) == 0 &&
		len(h.PreCompact) == 0 &&
		len(h.PostCompact) == 0 &&
		len(h.SubagentStart) == 0 &&
		len(h.SubagentStop) == 0 &&
		len(h.Stop) == 0 &&
		len(h.StopFailure) == 0
}

// MergeHooks returns a single HooksConfig containing every entry from every
// layer, in the order the layers are given, per event. The merge is additive:
// every layer runs for the same event; the dispatcher's first-block-wins rule
// still applies, but every matching hook still executes.
//
// The documented order is global settings, then plugins in name order, then the
// workspace. A workspace cannot remove a global hook, which is what makes the
// global layer usable as an operator control — and the same is true of a plugin.
//
// Mutating the returned config does not affect the inputs.
func MergeHooks(layers ...HooksConfig) HooksConfig {
	var out HooksConfig
	for _, l := range layers {
		out.SessionStart = concatEntries(out.SessionStart, l.SessionStart)
		out.SessionEnd = concatEntries(out.SessionEnd, l.SessionEnd)
		out.UserPromptSubmit = concatEntries(out.UserPromptSubmit, l.UserPromptSubmit)
		out.PreToolUse = concatEntries(out.PreToolUse, l.PreToolUse)
		out.PostToolUse = concatEntries(out.PostToolUse, l.PostToolUse)
		out.PostToolUseFailure = concatEntries(out.PostToolUseFailure, l.PostToolUseFailure)
		out.Notification = concatEntries(out.Notification, l.Notification)
		out.PreCompact = concatEntries(out.PreCompact, l.PreCompact)
		out.PostCompact = concatEntries(out.PostCompact, l.PostCompact)
		out.SubagentStart = concatEntries(out.SubagentStart, l.SubagentStart)
		out.SubagentStop = concatEntries(out.SubagentStop, l.SubagentStop)
		out.Stop = concatEntries(out.Stop, l.Stop)
		out.StopFailure = concatEntries(out.StopFailure, l.StopFailure)
	}
	return out
}

// PluginHooks is the plugin layer's contribution, already concatenated in name
// order, plus what loading it noticed.
type PluginHooks struct {
	Config   HooksConfig
	Findings []plugin.Finding
}

// ResolvePluginHooks reads every plugin's hooks.yaml in name order.
//
// A plugin whose file will not parse contributes nothing and is reported: hook
// configuration fails open by design, and refusing to start the agent over one
// plugin's typo would be a worse failure than running without its hooks.
func ResolvePluginHooks(plugins []DiscoveredPlugin) PluginHooks {
	var out PluginHooks
	var layers []HooksConfig
	for _, p := range loadablePlugins(plugins) {
		cfg, err := loadHooksFile(filepath.Join(p.Path, "hooks.yaml"))
		if err != nil {
			out.Findings = append(out.Findings, plugin.Finding{
				Severity: plugin.SeverityError, Field: p.Name(), Plugins: []string{p.Name()},
				Message: fmt.Sprintf("hooks.yaml: %v", err),
			})
			continue
		}
		if cfg.IsEmpty() {
			continue
		}
		expandHookConfig(&cfg, PluginRootFor(p))
		layers = append(layers, cfg)
	}
	out.Config = MergeHooks(layers...)
	return out
}

// expandHookConfig resolves PluginVarRoot everywhere a string appears in one
// plugin's hooks. The config was just parsed for this plugin alone, so it is
// mutated in place.
func expandHookConfig(cfg *HooksConfig, root string) {
	cfg.eachEventSlice(func(entries *[]HookEntry) {
		for i := range *entries {
			e := &(*entries)[i]
			e.Command = expandPluginVar(e.Command, root)
			e.URL = expandPluginVar(e.URL, root)
			e.Prompt = expandPluginVar(e.Prompt, root)
			for k, v := range e.Headers {
				e.Headers[k] = expandPluginVar(v, root)
			}
			for k, v := range e.Input {
				e.Input[k] = expandPluginVarAny(v, root)
			}
		}
	})
}

// eachEventSlice applies fn to every event's entry slice.
func (h *HooksConfig) eachEventSlice(fn func(*[]HookEntry)) {
	fn(&h.SessionStart)
	fn(&h.SessionEnd)
	fn(&h.UserPromptSubmit)
	fn(&h.PreToolUse)
	fn(&h.PostToolUse)
	fn(&h.PostToolUseFailure)
	fn(&h.Notification)
	fn(&h.PreCompact)
	fn(&h.PostCompact)
	fn(&h.SubagentStart)
	fn(&h.SubagentStop)
	fn(&h.Stop)
	fn(&h.StopFailure)
}

func concatEntries(a, b []HookEntry) []HookEntry {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make([]HookEntry, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}
