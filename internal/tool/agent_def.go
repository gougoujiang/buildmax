package tool

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// SubAgentDef represents one parsed sub-agent definition file.
// The file uses YAML-like frontmatter (between --- delimiters) for metadata
// and the body after the closing --- as the system prompt.
type SubAgentDef struct {
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

// LoadAgentDefs reads all files from dir, parses each as an agent definition,
// and returns the valid definitions sorted alphabetically by Name.
// If dir does not exist, returns (nil, nil) — not an error.
// Individual files that fail to parse are skipped with a log warning.
func LoadAgentDefs(dir string) ([]SubAgentDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read agent defs directory: %w", err)
	}

	var defs []SubAgentDef
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
func parseAgentDef(data []byte) (SubAgentDef, error) {
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "---") {
		return SubAgentDef{}, errors.New("missing opening --- frontmatter delimiter")
	}

	// Remove the leading ---
	rest := content[3:]

	// Find the closing ---
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return SubAgentDef{}, errors.New("missing closing --- frontmatter delimiter")
	}

	frontmatterBlock := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:]) // skip past "\n---"

	kv := parseFrontmatter(frontmatterBlock)

	name := strings.TrimSpace(kv["name"])
	if name == "" {
		return SubAgentDef{}, errors.New("required field 'name' is missing or empty")
	}

	description := strings.TrimSpace(kv["description"])
	if description == "" {
		return SubAgentDef{}, errors.New("required field 'description' is missing or empty")
	}

	toolsRaw := strings.TrimSpace(kv["tools"])
	if toolsRaw == "" {
		return SubAgentDef{}, errors.New("required field 'tools' is missing or empty")
	}
	toolNames := splitAndTrim(toolsRaw, ",")
	if len(toolNames) == 0 {
		return SubAgentDef{}, errors.New("'tools' field has no valid entries")
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

	return SubAgentDef{
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

// AgentDefResolution is what scanning every source produced: the definitions
// that load, what a higher layer shadowed, and the collisions that stopped a
// name from loading at all.
type AgentDefResolution struct {
	Defs     []SubAgentDef
	Shadowed []plugin.Shadowed
	Findings []plugin.Finding
}

// ResolveAgentDefs scans priority-ordered sources and reduces them to one
// definition per name. Sources come from internal/config: workspace, then
// global, then each plugin in name order.
func ResolveAgentDefs(sources []plugin.Source) (AgentDefResolution, error) {
	var cands []candidate[SubAgentDef]
	for _, src := range sources {
		defs, err := LoadAgentDefs(src.Dir)
		if err != nil {
			return AgentDefResolution{}, err
		}
		for _, d := range defs {
			cands = append(cands, candidate[SubAgentDef]{name: d.Name, origin: src.Origin, value: d})
		}
	}
	r := resolveCandidates(cands, "subagent", func(d SubAgentDef, o plugin.Origin) SubAgentDef {
		d.Origin = o
		return d
	})
	return AgentDefResolution{Defs: r.values, Shadowed: r.shadowed, Findings: r.findings}, nil
}

// LoadAgentDefsFromPaths loads agent definitions from multiple directories in priority order.
// Directories are scanned in order; if two directories contain a definition with the same Name,
// the first one wins (project-level overrides global-level). Missing directories are gracefully
// skipped. This is the unlabelled form, for a caller with no layering to express.
func LoadAgentDefsFromPaths(dirs []string) ([]SubAgentDef, error) {
	sources := make([]plugin.Source, 0, len(dirs))
	for _, dir := range dirs {
		sources = append(sources, plugin.Source{Dir: dir})
	}
	res, err := ResolveAgentDefs(sources)
	if err != nil {
		return nil, err
	}
	return res.Defs, nil
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
