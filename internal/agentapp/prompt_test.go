package agentapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAgentsMd(t *testing.T) {
	t.Run("file_present", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, AgentsMdFilename)
		want := "# Project rules\n- Use pnpm.\n- Run tests before commit."
		if err := os.WriteFile(path, []byte("\n  "+want+"  \n"), 0644); err != nil {
			t.Fatal(err)
		}
		got, err := ReadAgentsMd(dir)
		if err != nil {
			t.Fatalf("ReadAgentsMd: %v", err)
		}
		if got != want {
			t.Errorf("content = %q, want %q", got, want)
		}
	})

	t.Run("file_absent", func(t *testing.T) {
		dir := t.TempDir()
		got, err := ReadAgentsMd(dir)
		if err != nil {
			t.Fatalf("ReadAgentsMd: %v", err)
		}
		if got != "" {
			t.Errorf("content = %q, want empty", got)
		}
	})

	t.Run("directory_nonexistent", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nonexistent")
		got, err := ReadAgentsMd(dir)
		if err != nil {
			t.Fatalf("ReadAgentsMd (nonexistent dir): err = %v, want nil (ErrNotExist is treated as no file)", err)
		}
		if got != "" {
			t.Errorf("content = %q, want empty", got)
		}
	})
}

// TestBuildEffectiveSystemPrompt_NoCompactionBlock pins the ownership rule: RunLoop reads the
// stored summary through agent.CompactionHistory and renders the block itself. A caller that
// also folds the summary into the base prompt puts two compaction blocks in front of the
// model, which is what used to happen on every run after the first in-run compaction.
func TestBuildEffectiveSystemPrompt_NoCompactionBlock(t *testing.T) {
	got := BuildEffectiveSystemPrompt(t.TempDir(), "test-model", "", PromptCapabilities{})
	if strings.Contains(got, "context_compaction") {
		t.Error("base system prompt renders a compaction block; RunLoop is its only renderer")
	}
}
