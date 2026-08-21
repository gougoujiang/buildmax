package tool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// SkillEntry holds metadata for one discovered skill.
type SkillEntry struct {
	Name        string // skill identifier (directory name)
	Description string // short description extracted from SKILL.md
	Path        string // absolute path to the SKILL.md file
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

	slog.Info("skill: invoked", "skill", skillName, "content_len", len(content))
	return content, nil
}

// DiscoverSkillEntries scans search paths for subdirectories containing SKILL.md.
// First-path-wins on name conflicts. Returns skills sorted alphabetically by name.
// This is the single discovery implementation used by NewSkill and other callers
// that need a listing (e.g. TUI /skills).
func DiscoverSkillEntries(searchPaths []string) []SkillEntry {
	seen := make(map[string]bool)
	var skills []SkillEntry

	for _, dir := range searchPaths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				slog.Debug("skill: search path does not exist, skipping", "path", dir)
			} else {
				slog.Warn("skill: error reading search path, skipping", "path", dir, "err", err)
			}
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if seen[name] {
				continue
			}

			skillPath := filepath.Join(dir, name, "SKILL.md")
			info, err := os.Stat(skillPath)
			if err != nil || info.IsDir() {
				continue
			}

			data, err := os.ReadFile(skillPath)
			if err != nil {
				slog.Warn("skill: error reading SKILL.md, skipping", "path", skillPath, "err", err)
				continue
			}

			absPath, err := filepath.Abs(skillPath)
			if err != nil {
				slog.Warn("skill: error resolving path, skipping", "path", skillPath, "err", err)
				continue
			}

			desc := extractDescription(data)
			skills = append(skills, SkillEntry{
				Name:        name,
				Description: desc,
				Path:        absPath,
			})
			seen[name] = true
			slog.Info("skill: discovered", "name", name, "path", absPath)
		}
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
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
