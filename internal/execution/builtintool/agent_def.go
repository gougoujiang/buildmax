package builtintool

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AgentDef represents one parsed user-defined agent definition file.
// The file uses YAML-like frontmatter (between --- delimiters) for metadata
// and the body after the closing --- as the system prompt.
type AgentDef struct {
	Name         string   // agent type name (used as subagent_type value)
	Description  string   // LLM-readable description
	ToolNames    []string // tool names parsed from the "tools" field
	SystemPrompt string   // body text used as the sub-agent system prompt
	Model        string   // model hint (parsed, not used yet)
	Color        string   // color hint (parsed, not used yet)
}

// LoadAgentDefs reads all files from dir, parses each as an agent definition,
// and returns the valid definitions sorted alphabetically by Name.
// If dir does not exist, returns (nil, nil) — not an error.
// Individual files that fail to parse are skipped with a log warning.
func LoadAgentDefs(dir string) ([]AgentDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read agent defs directory: %w", err)
	}

	var defs []AgentDef
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("skip agent def: read error", "file", entry.Name(), "err", err)
			continue
		}
		def, err := parseAgentDef(data)
		if err != nil {
			slog.Warn("skip agent def: parse error", "file", entry.Name(), "err", err)
			continue
		}
		defs = append(defs, def)
	}

	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Name < defs[j].Name
	})
	return defs, nil
}

// parseAgentDef parses a single agent definition from file content.
// Expected format: YAML-like frontmatter between --- delimiters, then body.
func parseAgentDef(data []byte) (AgentDef, error) {
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "---") {
		return AgentDef{}, errors.New("missing opening --- frontmatter delimiter")
	}

	// Remove the leading ---
	rest := content[3:]

	// Find the closing ---
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return AgentDef{}, errors.New("missing closing --- frontmatter delimiter")
	}

	frontmatterBlock := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:]) // skip past "\n---"

	kv := parseFrontmatter(frontmatterBlock)

	name := strings.TrimSpace(kv["name"])
	if name == "" {
		return AgentDef{}, errors.New("required field 'name' is missing or empty")
	}

	description := strings.TrimSpace(kv["description"])
	if description == "" {
		return AgentDef{}, errors.New("required field 'description' is missing or empty")
	}

	toolsRaw := strings.TrimSpace(kv["tools"])
	if toolsRaw == "" {
		return AgentDef{}, errors.New("required field 'tools' is missing or empty")
	}
	toolNames := splitAndTrim(toolsRaw, ",")
	if len(toolNames) == 0 {
		return AgentDef{}, errors.New("'tools' field has no valid entries")
	}

	systemPrompt := body
	if systemPrompt == "" {
		systemPrompt = description // fallback so sub-agent always has a prompt
	}

	return AgentDef{
		Name:         name,
		Description:  description,
		ToolNames:    toolNames,
		SystemPrompt: systemPrompt,
		Model:        strings.TrimSpace(kv["model"]),
		Color:        strings.TrimSpace(kv["color"]),
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

// LoadAgentDefsFromPaths loads agent definitions from multiple directories in priority order.
// Directories are scanned in order; if two directories contain a definition with the same Name,
// the first one wins (project-level overrides global-level). Missing directories are gracefully
// skipped (same behavior as LoadAgentDefs). Returns the merged list sorted alphabetically by Name.
func LoadAgentDefsFromPaths(dirs []string) ([]AgentDef, error) {
	seen := make(map[string]bool)
	var merged []AgentDef

	for _, dir := range dirs {
		defs, err := LoadAgentDefs(dir)
		if err != nil {
			return nil, err
		}
		for _, d := range defs {
			if seen[d.Name] {
				slog.Debug("skip duplicate agent def", "name", d.Name, "dir", dir)
				continue
			}
			seen[d.Name] = true
			merged = append(merged, d)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Name < merged[j].Name
	})
	return merged, nil
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
