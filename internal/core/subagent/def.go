// Package subagent holds the shape of a subagent definition file and the rules
// its frontmatter must satisfy.
//
// It sits in core because two callers that cannot share anything higher both
// need it: loading definitions for a run, and inspecting an uploaded plugin
// package to report the subagents it contributes, which internal/server does.
package subagent

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// Def represents one parsed sub-agent definition file.
// The file uses YAML-like frontmatter (between --- delimiters) for metadata
// and the body after the closing --- as the system prompt.
type Def struct {
	Name          string   // agent type name (used as subagent_type value)
	Description   string   // LLM-readable description
	ToolNames     []string // tool names parsed from the "tools" field
	SystemPrompt  string   // body text used as the sub-agent system prompt
	Model         string   // model name to use for this agent type; "" = runner default
	MaxIterations int      // iteration cap; 0 = defaultSubAgentMaxIter (50)
	Color         string   // color hint (parsed, reserved for UI)
	// Origin is the layer this definition was found in, so status can show a
	// plugin's contribution and anything that shadowed it.
	Origin plugin.Origin
}

// ParseDef parses a single agent definition from file content.
// Expected format: YAML-like frontmatter between --- delimiters, then body.
func ParseDef(data []byte) (Def, error) {
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "---") {
		return Def{}, errors.New("missing opening --- frontmatter delimiter")
	}

	// Remove the leading ---
	rest := content[3:]

	// Find the closing ---
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return Def{}, errors.New("missing closing --- frontmatter delimiter")
	}

	frontmatterBlock := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:]) // skip past "\n---"

	kv := parseFrontmatter(frontmatterBlock)

	name := strings.TrimSpace(kv["name"])
	if name == "" {
		return Def{}, errors.New("required field 'name' is missing or empty")
	}

	description := strings.TrimSpace(kv["description"])
	if description == "" {
		return Def{}, errors.New("required field 'description' is missing or empty")
	}

	toolsRaw := strings.TrimSpace(kv["tools"])
	if toolsRaw == "" {
		return Def{}, errors.New("required field 'tools' is missing or empty")
	}
	toolNames := splitAndTrim(toolsRaw, ",")
	if len(toolNames) == 0 {
		return Def{}, errors.New("'tools' field has no valid entries")
	}

	systemPrompt := body
	if systemPrompt == "" {
		systemPrompt = description // fallback so sub-agent always has a prompt
	}

	var maxIter int
	if s := strings.TrimSpace(kv["max_iterations"]); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxIter = n
		}
	}

	return Def{
		Name:          name,
		Description:   description,
		ToolNames:     toolNames,
		SystemPrompt:  systemPrompt,
		Model:         strings.TrimSpace(kv["model"]),
		MaxIterations: maxIter,
		Color:         strings.TrimSpace(kv["color"]),
	}, nil
}

// parseFrontmatter parses a block of key: value lines into a map.
// Lines without a colon or with an empty key are skipped.
func parseFrontmatter(block string) map[string]string {
	kv := make(map[string]string)
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		kv[key] = value
	}
	return kv
}

// splitAndTrim splits s by sep and returns trimmed, non-empty parts.
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
