package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"

	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// WorkspaceHooksPath returns the per-workspace hooks config path
// (<workspace>/.buildmax/hooks.yaml). It does not check existence.
func WorkspaceHooksPath(workspace string) string {
	return filepath.Join(workspace, ".buildmax", "hooks.yaml")
}

// LoadWorkspaceHooks reads <workspace>/.buildmax/hooks.yaml. A missing file
// is not an error — returns (corehook.Config{}, nil) so callers can merge
// unconditionally. A malformed file is reported as an error so misconfig
// surfaces at startup instead of silently dropping rules.
//
// The file shape mirrors the "hooks:" block of settings.yaml; the top-level
// keys are pre_tool_use / post_tool_use / pre_compact / post_compact /
// run_end (the same snake_case event keys).
func LoadWorkspaceHooks(workspace string) (corehook.Config, error) {
	if workspace == "" {
		return corehook.Config{}, nil
	}
	return loadHooksFile(WorkspaceHooksPath(workspace))
}

// loadHooksFile reads one hooks.yaml. A missing file is not an error; a
// malformed one is, so misconfiguration surfaces instead of silently dropping
// rules.
func loadHooksFile(path string) (corehook.Config, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return corehook.Config{}, nil
		}
		return corehook.Config{}, fmt.Errorf("stat hooks %q: %w", path, err)
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return corehook.Config{}, fmt.Errorf("read hooks %q: %w", path, err)
	}
	var cfg corehook.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return corehook.Config{}, fmt.Errorf("parse hooks %q: %w", path, err)
	}
	return cfg, nil
}

// MergeHooks returns a single corehook.Config containing every entry from every
// layer, in the order the layers are given, per event. The merge is additive:
// every layer runs for the same event; the dispatcher's first-block-wins rule
// still applies, but every matching hook still executes.
//
// The documented order is global settings, then plugins in name order, then the
// workspace. A workspace cannot remove a global hook, which is what makes the
// global layer usable as an operator control — and the same is true of a plugin.
//
// Mutating the returned config does not affect the inputs.
func MergeHooks(layers ...corehook.Config) corehook.Config {
	var out corehook.Config
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
		out.WorktreeCreate = concatEntries(out.WorktreeCreate, l.WorktreeCreate)
		out.WorktreeRemove = concatEntries(out.WorktreeRemove, l.WorktreeRemove)
		out.CwdChanged = concatEntries(out.CwdChanged, l.CwdChanged)
	}
	return out
}

// PluginHooks is the plugin layer's contribution, already concatenated in name
// order, plus what loading it noticed.
type PluginHooks struct {
	Config   corehook.Config
	Findings []plugin.Finding
}

// ResolvePluginHooks reads every plugin's hooks.yaml in name order.
//
// A plugin whose file will not parse contributes nothing and is reported: hook
// configuration fails open by design, and refusing to start the agent over one
// plugin's typo would be a worse failure than running without its hooks.
func ResolvePluginHooks(plugins []DiscoveredPlugin) PluginHooks {
	var out PluginHooks
	var layers []corehook.Config
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
func expandHookConfig(cfg *corehook.Config, root string) {
	cfg.EachEntry(func(e *corehook.Entry) {
		e.Command = expandPluginVar(e.Command, root)
		e.URL = expandPluginVar(e.URL, root)
		e.Prompt = expandPluginVar(e.Prompt, root)
		for k, v := range e.Headers {
			e.Headers[k] = expandPluginVar(v, root)
		}
		for k, v := range e.Input {
			e.Input[k] = expandPluginVarAny(v, root)
		}
	})
}

func concatEntries(a, b []corehook.Entry) []corehook.Entry {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make([]corehook.Entry, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}
