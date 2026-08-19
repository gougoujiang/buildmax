package agentapp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

const testPromptText = "You are a law consultant.\n\n## Invariants\n- Name the jurisdiction.\n"

// TestBuildSystemPrompt_AdditionalTextIsTheLastLayer pins the layer order. The additional text
// is the most specific thing the run was told, and every layer is additive: the runtime prompt
// is never replaced, because losing it would strip the tool-usage conventions and look like a
// bad model.
func TestBuildSystemPrompt_AdditionalTextIsTheLastLayer(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, AgentsMdFilename), []byte("project rules here"), 0o644); err != nil {
		t.Fatalf("write workspace AGENTS.md: %v", err)
	}

	got := BuildEffectiveSystemPrompt(ws, "test-model", testPromptText)

	if !strings.HasPrefix(got, DefaultSystemPrompt) {
		t.Error("the runtime prompt is no longer the first layer")
	}
	extraAt := strings.Index(got, "You are a law consultant.")
	wsAt := strings.Index(got, "project rules here")
	if extraAt < 0 {
		t.Fatal("additional text missing from the prompt")
	}
	if wsAt < 0 || extraAt < wsAt {
		t.Errorf("additional text must follow the workspace layer: at %d, workspace at %d", extraAt, wsAt)
	}
}

func TestBuildSystemPrompt_NoAdditionalTextAddsNothing(t *testing.T) {
	ws := t.TempDir()
	bare := BuildEffectiveSystemPrompt(ws, "m", "")
	if strings.Contains(bare, "# Additional instructions") {
		t.Error("empty additional text still rendered its heading")
	}
}

func TestBuildSystemPromptWithLayers_ReportsWhatItLoaded(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, AgentsMdFilename), []byte("project rules"), 0o644); err != nil {
		t.Fatalf("write workspace AGENTS.md: %v", err)
	}

	_, layers := BuildSystemPromptWithLayers(ws, "m", testPromptText)

	var names []string
	for _, l := range layers {
		names = append(names, l.Name)
		if l.Chars <= 0 {
			t.Errorf("layer %q reports %d chars", l.Name, l.Chars)
		}
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "runtime") || !strings.Contains(joined, "workspace_agents_md") || !strings.Contains(joined, "additional_system_prompt") {
		t.Errorf("layers = %v, want at least runtime, workspace_agents_md and additional_system_prompt", names)
	}
}

func TestValidateAdditionalSystemPrompt(t *testing.T) {
	if err := ValidateAdditionalSystemPrompt(strings.Repeat("x", MaxAdditionalSystemPromptChars)); err != nil {
		t.Errorf("text at the limit was rejected: %v", err)
	}
	err := ValidateAdditionalSystemPrompt(strings.Repeat("x", MaxAdditionalSystemPromptChars+1))
	if err == nil {
		t.Fatal("over-limit text was accepted")
	}
	// The slot has no degradation path, so the failure has to say what to cut.
	if !strings.Contains(err.Error(), "limit is") {
		t.Errorf("error does not name the limit: %v", err)
	}
}

// TestEffectiveAdditionalPrompt covers the resolution rule: a configured value wins, which is
// what makes an edited Portal agent definition take effect on the next run; with none
// configured, a resumed session keeps the identity it already ran under instead of silently
// losing it.
func TestEffectiveAdditionalPrompt(t *testing.T) {
	sess := NewSessionContext(session.NewSession(""), "m")
	sess.AdditionalSystemPrompt = "stored text"

	configured := &AgentApp{additionalSystemPrompt: "configured text"}
	if got := configured.effectiveAdditionalPrompt(sess); got != "configured text" {
		t.Errorf("configured = %q, want the configured value to win", got)
	}

	bare := &AgentApp{}
	if got := bare.effectiveAdditionalPrompt(sess); got != "stored text" {
		t.Errorf("resumed = %q, want the session's stored text", got)
	}
	if got := bare.effectiveAdditionalPrompt(nil); got != "" {
		t.Errorf("no session and no config = %q, want empty", got)
	}
}

func TestExtractInvariantsFromPromptText(t *testing.T) {
	got := agent.ExtractInvariants(testPromptText)
	if got != "- Name the jurisdiction." {
		t.Errorf("invariants = %q", got)
	}
	if agent.ExtractInvariants("prompt text with no invariants section") != "" {
		t.Error("text without the section reported invariants")
	}
}

// makeAppWithPrompt builds an AgentApp configured with the given additional system prompt and a
// stub model, then swaps in a client that records what it was sent.
func makeAppWithPrompt(t *testing.T, extra string) (*AgentApp, *scriptedClient) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("BUILDMAX_HOME", home)
	settings := "log_level: error\nmodels:\n  - model: stub\n    name: stub\n    api_key: x\n"
	if err := os.WriteFile(filepath.Join(home, "settings.yaml"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := NewAgentApp(AppConfig{WorkspaceDir: t.TempDir(), AdditionalSystemPrompt: extra})
	if err != nil {
		t.Fatalf("NewAgentApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	client := &scriptedClient{replies: []scriptedReply{{content: "2"}}}
	app.llmClients.mu.Lock()
	app.llmClients.clients["stub"] = client
	app.llmClients.mu.Unlock()
	return app, client
}

// TestRunPrompt_SendsAdditionalSystemPrompt is the end-to-end check the CLI flags exist for: the
// text a user supplies has to reach the system message of the actual model call, not merely be
// stored somewhere on the way.
func TestRunPrompt_SendsAdditionalSystemPrompt(t *testing.T) {
	app, client := makeAppWithPrompt(t, "answer 24 for 1+1")

	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	if _, err := app.RunPrompt(context.Background(), sess, "what is 1+1", nil, nil, nil); err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}

	if len(client.sent) == 0 {
		t.Fatal("no model call was made")
	}
	first := client.sent[0]
	if len(first) == 0 || first[0].Role != "system" {
		t.Fatalf("first message is not the system prompt: %+v", first)
	}
	if !strings.Contains(first[0].Content, "answer 24 for 1+1") {
		t.Errorf("the additional system prompt never reached the model:\n%s", first[0].Content)
	}
	if !strings.Contains(first[0].Content, DefaultSystemPrompt) {
		t.Error("the runtime prompt was replaced rather than appended to")
	}
}

// TestRunPrompt_RecordsWhatItRanUnder asserts the session keeps the text the run used, so a
// resume without the flag does not silently drop it.
func TestRunPrompt_RecordsWhatItRanUnder(t *testing.T) {
	app, _ := makeAppWithPrompt(t, "answer 24 for 1+1")

	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	if _, err := app.RunPrompt(context.Background(), sess, "what is 1+1", nil, nil, nil); err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}

	if sess.AdditionalSystemPrompt != "answer 24 for 1+1" {
		t.Errorf("session recorded %q", sess.AdditionalSystemPrompt)
	}
}
