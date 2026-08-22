package tool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// SkillEntry holds metadata for one discovered skill.
type SkillEntry struct {
	Name        string // skill identifier (directory name)
	Description string // short description extracted from SKILL.md
	Path        string // absolute path to the SKILL.md file
	// Origin is the layer this skill was found in. Status surfaces it so a
	// plugin's contribution is visible, and so is anything that shadowed it.
	Origin plugin.Origin
}

// SkillTool is an agent tool that discovers and invokes skills from disk.
// It implements agent.Tool.
type SkillTool struct {
	skills []SkillEntry          // ordered list for deterministic description
	byName map[string]SkillEntry // fast lookup by skill name
}

// NewSkill creates a SkillTool that discovers skills from the given search paths.
// Each search path is scanned one level deep for subdirectories containing a SKILL.md file.
// Missing directories are silently skipped. If multiple search paths contain a skill
// with the same name, the first one found wins (search paths are priority-ordered).
func NewSkill(searchPaths []string) *SkillTool {
	return NewSkillFromEntries(DiscoverSkillEntries(searchPaths))
}

// NewSkillFromEntries creates a SkillTool from pre-discovered skill entries.
// The provided entries are copied so callers can treat the returned tool as immutable.
func NewSkillFromEntries(entries []SkillEntry) *SkillTool {
	skills := cloneSkillEntries(entries)
	byName := make(map[string]SkillEntry, len(skills))
	for _, s := range skills {
		byName[s.Name] = s
	}
	return &SkillTool{skills: skills, byName: byName}
}

// Name returns the tool name for the LLM.
// Access implements llm.AccessDeclarer.
func (s *SkillTool) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }

func (s *SkillTool) Name() string { return ToolNameSkill }

// Description returns a static preamble followed by a dynamic listing of discovered skills.
func (s *SkillTool) Description() string {
	var b strings.Builder
	b.WriteString("Invoke a skill to get specialized instructions for a task. ")
	b.WriteString("Skills provide domain-specific workflows and knowledge. ")
	b.WriteString("When a skill is relevant, invoke this tool IMMEDIATELY as your first action.\n\n")
	if len(s.skills) == 0 {
		b.WriteString("No skills are currently available.")
		return b.String()
	}
	b.WriteString("Available skills:\n")
	for _, entry := range s.skills {
		fmt.Fprintf(&b, "- %s: %s\n", entry.Name, entry.Description)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// Parameters returns the OpenAI-style JSON schema for the tool arguments.
func (s *SkillTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"skill": map[string]any{
				"type":        "string",
				"description": "The skill name to invoke (e.g. \"vibe\", \"commit\")",
			},
			"args": map[string]any{
				"type":        "string",
				"description": "Optional arguments for the skill",
			},
		},
		"required": []string{"skill"},
	}
}

// Execute looks up the requested skill, reads its SKILL.md file, and returns the content.
// If args is provided, it is prepended as context.
func (s *SkillTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	skillName, _ := args["skill"].(string)
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return "", errors.New("skill is required")
	}

	entry, ok := s.byName[skillName]
	if !ok {
		available := make([]string, 0, len(s.skills))
		for _, sk := range s.skills {
			available = append(available, sk.Name)
		}
		if len(available) == 0 {
			return "", fmt.Errorf("unknown skill %q; no skills are available", skillName)
		}
		return "", fmt.Errorf("unknown skill %q; available skills: %s", skillName, strings.Join(available, ", "))
	}

	data, err := os.ReadFile(entry.Path)
	if err != nil {
		return "", fmt.Errorf("read skill file: %w", err)
	}

	// Prepend the skill's base directory so the LLM can locate
	// additional files (scripts/, references/, assets/, etc.).
	baseDir := filepath.Dir(entry.Path)
	var b strings.Builder
	b.WriteString("Skill directory: " + baseDir + "\n\n")

	skillArgs, _ := args["args"].(string)
	skillArgs = strings.TrimSpace(skillArgs)
	if skillArgs != "" {
		b.WriteString("Arguments: " + skillArgs + "\n\n")
	}

	b.Write(data)
	content := b.String()

	skillLog().Info("invoked", "skill", skillName, "content_len", len(content))
	return content, nil
}

// SkillResolution is what scanning every source produced: the skills that load,
// the definitions a higher layer shadowed, and the collisions that stopped a
// name from loading at all.
type SkillResolution struct {
	Entries  []SkillEntry
	Shadowed []Shadowed
	Findings []plugin.Finding
}

// ResolveSkills scans priority-ordered sources and reduces them to one skill per
// name. Sources come from internal/config: workspace, then global, then each
// plugin in name order.
func ResolveSkills(sources []plugin.Source) SkillResolution {
	var cands []candidate[SkillEntry]
	for _, src := range sources {
		for _, e := range scanSkillDir(src.Dir) {
			cands = append(cands, candidate[SkillEntry]{name: e.Name, origin: src.Origin, value: e})
		}
	}
	r := resolveCandidates(cands, "skill", func(e SkillEntry, o plugin.Origin) SkillEntry {
		e.Origin = o
		return e
	})
	return SkillResolution{Entries: r.values, Shadowed: r.shadowed, Findings: r.findings}
}

// DiscoverSkillEntries scans search paths for subdirectories containing SKILL.md.
// First-path-wins on name conflicts. Returns skills sorted alphabetically by name.
// This is the unlabelled form, for a caller with no layering to express.
func DiscoverSkillEntries(searchPaths []string) []SkillEntry {
	sources := make([]plugin.Source, 0, len(searchPaths))
	for _, dir := range searchPaths {
		sources = append(sources, plugin.Source{Dir: dir})
	}
	return ResolveSkills(sources).Entries
}

// scanSkillDir reads one directory. Every readable skill is returned, including
// names another source also defines: resolution decides what that means, and it
// cannot decide from a list the losers were already dropped from.
func scanSkillDir(dir string) []SkillEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			skillLog().Debug("search path does not exist, skipping", "path", dir)
		} else {
			skillLog().Warn("error reading search path, skipping", "path", dir, "err", err)
		}
		return nil
	}

	var skills []SkillEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		skillPath := filepath.Join(dir, name, "SKILL.md")
		info, err := os.Stat(skillPath)
		if err != nil || info.IsDir() {
			continue
		}

		data, err := os.ReadFile(skillPath)
		if err != nil {
			skillLog().Warn("error reading SKILL.md, skipping", "path", skillPath, "err", err)
			continue
		}

		absPath, err := filepath.Abs(skillPath)
		if err != nil {
			skillLog().Warn("error resolving path, skipping", "path", skillPath, "err", err)
			continue
		}

		skills = append(skills, SkillEntry{
			Name:        name,
			Description: extractDescription(data),
			Path:        absPath,
		})
		skillLog().Info("discovered", "name", name, "path", absPath)
	}
	return skills
}

// extractDescription returns the first non-empty, non-heading line from SKILL.md content.
// If no suitable line is found, returns "(no description)".
func extractDescription(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		// Truncate very long descriptions for the listing.
		if len(line) > 200 {
			line = line[:200] + "..."
		}
		return line
	}
	return "(no description)"
}

func cloneSkillEntries(entries []SkillEntry) []SkillEntry {
	if len(entries) == 0 {
		return nil
	}
	cloned := make([]SkillEntry, len(entries))
	copy(cloned, entries)
	return cloned
}

// Identity belongs in an attr, not in every message string.
func skillLog() *slog.Logger { return slog.With("component", "skill") }
