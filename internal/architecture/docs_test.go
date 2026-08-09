package architecture_test

// Documentation constraints. These keep docs/ honest about things the code is
// the source of truth for: every relative link must resolve, every environment
// variable must be documented, and every LLM-facing tool name must appear in
// the user-facing tool guide.
//
// Conventions these enforce: docs/contribute/documentation.md.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/tool"
)

// markdownFiles returns every documentation file whose links are checked.
func markdownFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	docsDir := filepath.Join(root, "docs")
	err := filepath.WalkDir(docsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}
	for _, name := range []string{"README.md", "CONTRIBUTING.md", "ROADMAP.md", "AGENTS.md", "SECURITY.md"} {
		p := filepath.Join(root, name)
		if _, err := os.Stat(p); err == nil {
			files = append(files, p)
		}
	}
	return files
}

var markdownLinkRe = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)

// TestDocsLinksResolve fails when a relative markdown link points at a file
// that does not exist. Moving a document without updating its inbound links is
// the most common way documentation rots.
func TestDocsLinksResolve(t *testing.T) {
	root := repoRoot(t)
	for _, file := range markdownFiles(t, root) {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, m := range markdownLinkRe.FindAllStringSubmatch(string(body), -1) {
			target := strings.TrimSpace(m[1])
			if i := strings.Index(target, "#"); i >= 0 {
				target = target[:i]
			}
			if target == "" ||
				strings.HasPrefix(target, "http://") ||
				strings.HasPrefix(target, "https://") ||
				strings.HasPrefix(target, "mailto:") {
				continue
			}
			resolved := filepath.Join(filepath.Dir(file), target)
			if _, err := os.Stat(resolved); err != nil {
				rel, _ := filepath.Rel(root, file)
				t.Errorf("%s: broken link %q", rel, m[1])
			}
		}
	}
}

// TestEnvVarsDocumented fails when config.EnvVars gains a variable that
// docs/reference/configuration.md does not mention. env_spec.go is the source
// of truth; this keeps the reference table from silently falling behind it.
func TestEnvVarsDocumented(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "docs", "reference", "configuration.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read configuration reference: %v", err)
	}
	doc := string(body)
	for _, ev := range config.EnvVars {
		if !strings.Contains(doc, ev.Name) {
			t.Errorf("environment variable %s is in config.EnvVars but not in docs/reference/configuration.md", ev.Name)
		}
	}
}

// TestToolNamesDocumented fails when a tool name constant is missing from the
// user-facing tool guide. These exact strings are what users type into hook
// matchers and subagent "tools:" fields, so a rename that skips the docs
// silently breaks working configuration.
func TestToolNamesDocumented(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "docs", "guide", "tools.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tool guide: %v", err)
	}
	doc := string(body)
	names := []string{
		tool.ToolNameRead, tool.ToolNameWrite, tool.ToolNameEdit,
		tool.ToolNameGlob, tool.ToolNameGrep, tool.ToolNameBash,
		tool.ToolNameWebFetch, tool.ToolNameTodoWrite,
		tool.ToolNameSkill, tool.ToolNameTask,
		tool.ToolNameLoadMCPTools, tool.ToolNameCallMCPTool,
	}
	for _, name := range names {
		if !strings.Contains(doc, "`"+name+"`") {
			t.Errorf("tool %q is registered but not documented in docs/guide/tools.md", name)
		}
	}
}

// agentsMDPathRe matches repository paths written in backticks, e.g. `internal/config`.
var agentsMDPathRe = regexp.MustCompile("`((?:internal|cmd|docs|portal|gui|desktop|config-examples|deployment|setup|eval|scripts)/[A-Za-z0-9_./-]*)`")

// TestAgentsMDPathsExist fails when AGENTS.md cites a repository path that does
// not exist. AGENTS.md is loaded into every agent session, so a stale path there
// misleads more often than a stale path in any single document — and it has
// repeatedly been the source that other documents copied from.
func TestAgentsMDPathsExist(t *testing.T) {
	root := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	for _, m := range agentsMDPathRe.FindAllStringSubmatch(string(body), -1) {
		p := strings.TrimSuffix(m[1], "/")
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("AGENTS.md cites %q, which does not exist", m[1])
		}
	}
}

// TestAgentsMDRoutesExist fails when AGENTS.md lists an HTTP route the server
// does not register. The route list is a summary, so extra registered routes are
// fine — a documented route that does not exist is not.
func TestAgentsMDRoutesExist(t *testing.T) {
	root := repoRoot(t)
	routesSrc, err := os.ReadFile(filepath.Join(root, "internal", "server", "handlers", "routes.go"))
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}
	registered := regexp.MustCompile(`mux\.Handle(?:Func)?\("[A-Z]+ ([^"]+)"`).
		FindAllStringSubmatch(string(routesSrc), -1)
	paths := map[string]bool{"/healthz": true, "/openapi.json": true, "/swagger/": true}
	for _, m := range registered {
		paths[m[1]] = true
	}

	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	cited := regexp.MustCompile("`(?:[A-Z/]+ )?(/api/[^`,;]+)`").FindAllStringSubmatch(string(body), -1)
	for _, m := range cited {
		route := strings.TrimSpace(m[1])
		// Prose forms like /api/worker/* and /api/teams/{team_id}/... name a
		// group of routes rather than one, and have nothing to match against.
		if strings.HasSuffix(route, "*") || strings.HasSuffix(route, "...") {
			continue
		}
		if !paths[route] {
			t.Errorf("AGENTS.md documents route %q, which the server does not register", route)
		}
	}
}
