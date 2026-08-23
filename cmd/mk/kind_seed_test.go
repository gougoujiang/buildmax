package main

import (
	"strings"
	"testing"
)

// The catalog table is aligned with spaces and a model name may contain them,
// so the columns are read from the header rather than split on whitespace.
func TestParseCatalogIDs(t *testing.T) {
	output := strings.Join([]string{
		`time=2026-08-23T10:00:00Z level=INFO msg="connected to mysql"`,
		"ID                    NAME             PROVIDER           MODEL                 API URL                      ENABLED",
		"gsyt7at6cjfr33d73mta  GPT-5.6 Luna     openai_compatible  openai/gpt-5.6-luna   https://openrouter.ai/api/v1  yes",
		"a2b3c4d5e6f7g2h3i4j5  Qwen3 8B local   ollama             qwen3:8b              http://host.docker.internal:11434  no",
		"",
	}, "\n")

	ids := parseCatalogIDs(output)
	if len(ids) != 2 {
		t.Fatalf("parseCatalogIDs() returned %d rows, want 2: %v", len(ids), ids)
	}
	if got := ids["GPT-5.6 Luna"]; got != "gsyt7at6cjfr33d73mta" {
		t.Errorf("a name with a space resolved to %q", got)
	}
	if got := ids["Qwen3 8B local"]; got != "a2b3c4d5e6f7g2h3i4j5" {
		t.Errorf("second row resolved to %q", got)
	}
}

func TestParseCatalogIDsEmptyCatalog(t *testing.T) {
	if ids := parseCatalogIDs("The catalog is empty. Add one with: buildmax-server model add --help"); len(ids) != 0 {
		t.Errorf("an empty catalog parsed as %v", ids)
	}
}

func TestAddedModelPattern(t *testing.T) {
	output := strings.Join([]string{
		"Added model gsyt7at6cjfr33d73mta (GPT-5.6 Luna)",
		"",
		"To let a team use it, add an alias to server.yaml:",
		"",
		"  llm:",
		"    default_alias: default",
		"",
	}, "\n")
	match := addedModelPattern.FindStringSubmatch(output)
	if match == nil {
		t.Fatal("the add command's output no longer yields a model ID")
	}
	if match[1] != "gsyt7at6cjfr33d73mta" || match[2] != "GPT-5.6 Luna" {
		t.Errorf("matched %q and %q", match[1], match[2])
	}
}

// A local runtime's address is loopback in settings.local.yaml, which inside a
// pod is the pod. Everything else has to be left exactly as configured.
func TestKindReachableURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"http://localhost:11434", "http://" + kindHostAddress + ":11434"},
		{"http://127.0.0.1:1234/v1", "http://" + kindHostAddress + ":1234/v1"},
		{"http://localhost", "http://" + kindHostAddress},
		{"https://openrouter.ai/api/v1", "https://openrouter.ai/api/v1"},
		{"", ""},
	}
	for _, c := range cases {
		if got := kindReachableURL(c.in, "test"); got != c.want {
			t.Errorf("kindReachableURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderKindSeedAliases(t *testing.T) {
	rendered := renderKindSeedAliases([]kindSeedEntry{
		{alias: "openai-gpt-5-6-luna", source: "openai/gpt-5.6-luna", name: "GPT-5.6 Luna", id: "gsyt7at6cjfr33d73mta"},
		{alias: "qwen3-8b", source: "qwen3:8b", name: "Qwen3 8B", id: "a2b3c4d5e6f7g2h3i4j5"},
	})
	if !strings.Contains(rendered, "  default_alias: openai-gpt-5-6-luna\n") {
		t.Errorf("the first model is not the default alias:\n%s", rendered)
	}
	if !strings.Contains(rendered, "    qwen3-8b: a2b3c4d5e6f7g2h3i4j5  # qwen3:8b\n") {
		t.Errorf("an alias does not name the model it came from:\n%s", rendered)
	}
}

// server.yaml is read through viper, which splits a dotted key into a path: an
// alias carrying the dot from a model version stops the server from starting.
func TestAliasFromModelID(t *testing.T) {
	cases := map[string]string{
		"openai/gpt-5.6-luna":       "openai-gpt-5-6-luna",
		"anthropic/claude-haiku-4.5": "anthropic-claude-haiku-4-5",
		"qwen3:8b":                  "qwen3-8b",
		"z-ai/glm-5.3":              "z-ai-glm-5-3",
		"...":                       "",
	}
	for in, want := range cases {
		if got := aliasFromModelID(in); got != want {
			t.Errorf("aliasFromModelID(%q) = %q, want %q", in, got, want)
		}
		if strings.ContainsAny(aliasFromModelID(in), "./:") {
			t.Errorf("aliasFromModelID(%q) left a character viper reads as a path", in)
		}
	}
}

// A managed entry names a team alias, which is what seeding creates; there is
// nothing in it for a catalog to hold.
func TestDirectSettingsModelsDropsManaged(t *testing.T) {
	direct := directSettingsModels([]settingsModel{
		{id: "openai/gpt-5.6-luna", name: "Luna"},
		{id: "default", name: "Team Default", transport: "buildmax"},
	})
	if len(direct) != 1 || direct[0].id != "openai/gpt-5.6-luna" {
		t.Errorf("directSettingsModels() kept %+v", direct)
	}
}

func TestParseSettingsModelsReadsEveryCatalogField(t *testing.T) {
	text := `models:
  - model: anthropic/claude-sonnet-5
    name: Claude Sonnet 5
    provider: anthropic
    api_url: https://api.anthropic.com
    api_key: sk-test
    context_window: 1000000
    call_timeout: 300
    max_tokens: 8192
    reasoning: medium
    prompt_cache: true
    vision: true

  - model: default
    name: Team Default
    transport: buildmax
    server_url: http://localhost:5678
    team_id: tm_example
`
	models := parseSettingsModels(text)
	if len(models) != 2 {
		t.Fatalf("parsed %d models, want 2", len(models))
	}
	got := models[0]
	want := settingsModel{
		id: "anthropic/claude-sonnet-5", name: "Claude Sonnet 5", provider: "anthropic",
		apiURL: "https://api.anthropic.com", apiKey: "sk-test",
		contextWindow: 1000000, callTimeout: 300, maxTokens: 8192,
		reasoning: "medium", promptCache: true, vision: true,
	}
	if got != want {
		t.Errorf("parsed %+v, want %+v", got, want)
	}
	if !models[1].isManaged() {
		t.Error("a transport: buildmax entry did not parse as managed")
	}
}
