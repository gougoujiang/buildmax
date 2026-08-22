package tool

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/core/subagent"
)

// LoadAgentDefs reads all files from dir, parses each as an agent definition,
// and returns the valid definitions sorted alphabetically by Name.
// If dir does not exist, returns (nil, nil) — not an error.
// Individual files that fail to parse are skipped with a log warning.
func LoadAgentDefs(dir string) ([]subagent.Def, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read agent defs directory: %w", err)
	}

	var defs []subagent.Def
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
		def, err := subagent.ParseDef(data)
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

// AgentDefResolution is what scanning every source produced: the definitions
// that load, what a higher layer shadowed, and the collisions that stopped a
// name from loading at all.
type AgentDefResolution struct {
	Defs     []subagent.Def
	Shadowed []plugin.Shadowed
	Findings []plugin.Finding
}

// ResolveAgentDefs scans priority-ordered sources and reduces them to one
// definition per name. Sources come from internal/config: workspace, then
// global, then each plugin in name order.
func ResolveAgentDefs(sources []plugin.Source) (AgentDefResolution, error) {
	var cands []candidate[subagent.Def]
	for _, src := range sources {
		defs, err := LoadAgentDefs(src.Dir)
		if err != nil {
			return AgentDefResolution{}, err
		}
		for _, d := range defs {
			cands = append(cands, candidate[subagent.Def]{name: d.Name, origin: src.Origin, value: d})
		}
	}
	r := resolveCandidates(cands, "subagent", func(d subagent.Def, o plugin.Origin) subagent.Def {
		d.Origin = o
		return d
	})
	return AgentDefResolution{Defs: r.values, Shadowed: r.shadowed, Findings: r.findings}, nil
}

// LoadAgentDefsFromPaths loads agent definitions from multiple directories in priority order.
// Directories are scanned in order; if two directories contain a definition with the same Name,
// the first one wins (project-level overrides global-level). Missing directories are gracefully
// skipped. This is the unlabelled form, for a caller with no layering to express.
func LoadAgentDefsFromPaths(dirs []string) ([]subagent.Def, error) {
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
