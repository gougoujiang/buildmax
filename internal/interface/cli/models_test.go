package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// TestModelsLocalListsWhatTheDaemonHolds covers the column that decides whether
// a local model is usable at all — tool calling — and the paste-ready entry
// that follows, which must name a model that can.
func TestModelsLocalListsWhatTheDaemonHolds(t *testing.T) {
	daemon := fakeOllama{
		installed:    []string{"qwen3:8b"},
		capabilities: []string{"completion", "tools"},
		contextLen:   40_960,
	}
	url := daemon.start(t)

	var out bytes.Buffer
	if err := printOllamaModels(context.Background(), &out, url); err != nil {
		t.Fatalf("printOllamaModels: %v", err)
	}
	for _, want := range []string{"qwen3:8b", "40960", "tools", "provider: " + llm.ProviderOllama, url} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("listing missing %q:\n%s", want, out.String())
		}
	}
	// The suggested window is the daemon's answer capped, not its answer.
	if !strings.Contains(out.String(), "context_window: 32000") {
		t.Errorf("suggested entry should cap the window:\n%s", out.String())
	}
}

func TestModelsLocalSaysWhenNothingCanCallTools(t *testing.T) {
	daemon := fakeOllama{installed: []string{"embedder:latest"}, capabilities: []string{"completion"}}
	var out bytes.Buffer
	if err := printOllamaModels(context.Background(), &out, daemon.start(t)); err != nil {
		t.Fatalf("printOllamaModels: %v", err)
	}
	if !strings.Contains(out.String(), "None of these can call tools") {
		t.Errorf("listing should say why none of these will work:\n%s", out.String())
	}
}

func TestModelsLocalReportsAnAbsentDaemon(t *testing.T) {
	var out bytes.Buffer
	err := printOllamaModels(context.Background(), &out, "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected an error when nothing is listening")
	}
	if !strings.Contains(err.Error(), "ollama serve") {
		t.Errorf("error %q should say how to start the daemon", err)
	}
}

// TestModelDestinationForLocalEntry keeps "where does this send my prompts"
// answerable for an entry that names no URL.
func TestModelDestinationForLocalEntry(t *testing.T) {
	cfg := agentapp.ModelConfigFromEntry(config.ModelEntry{
		Model:    "qwen3:8b",
		Provider: llm.ProviderOllama,
	})
	if got := modelDestination(cfg); got != config.DefaultOllamaBaseURL {
		t.Errorf("destination = %q, want %q", got, config.DefaultOllamaBaseURL)
	}
}
