package taskrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp"
)

func TestBuildRunAgentsMdContent(t *testing.T) {
	const structureMarker = "## Run directory layout"
	const homeMarker = "**home/**"
	const artifactsMarker = "**artifacts/**"
	const globalMarker = "**global/**"

	tests := []struct {
		name          string
		setupRunHome  func(dir string) error // create runHome dir and optionally AGENTS.md
		wantStructure bool
		wantWorkspace string // optional workspace AGENTS.md content to expect (substring)
	}{
		{
			name: "no workspace AGENTS.md",
			setupRunHome: func(dir string) error {
				return os.MkdirAll(dir, 0755)
			},
			wantStructure: true,
			wantWorkspace: "",
		},
		{
			name: "workspace AGENTS.md present",
			setupRunHome: func(dir string) error {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
				path := filepath.Join(dir, agentapp.AgentsMdFilename)
				return os.WriteFile(path, []byte("# Project rules\n\nUse Python 3.10.\n"), 0644)
			},
			wantStructure: true,
			wantWorkspace: "Project rules",
		},
		{
			name: "workspace AGENTS.md empty file",
			setupRunHome: func(dir string) error {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
				path := filepath.Join(dir, agentapp.AgentsMdFilename)
				return os.WriteFile(path, []byte("   \n\n  "), 0644)
			},
			wantStructure: true,
			wantWorkspace: "", // trimmed to empty, so no extra section
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runHome := t.TempDir()
			if err := tt.setupRunHome(runHome); err != nil {
				t.Fatalf("setup: %v", err)
			}
			content, err := BuildRunAgentsMdContent(runHome)
			if err != nil {
				t.Fatalf("BuildRunAgentsMdContent: %v", err)
			}
			if tt.wantStructure {
				if !strings.Contains(content, structureMarker) {
					t.Errorf("content missing run directory structure section: %q", content)
				}
				if !strings.Contains(content, homeMarker) {
					t.Errorf("content missing home/ doc: %q", content)
				}
				if !strings.Contains(content, artifactsMarker) {
					t.Errorf("content missing artifacts/ doc: %q", content)
				}
				if !strings.Contains(content, globalMarker) {
					t.Errorf("content missing global/ doc: %q", content)
				}
			}
			if tt.wantWorkspace != "" {
				if !strings.Contains(content, tt.wantWorkspace) {
					t.Errorf("content missing workspace AGENTS.md content %q: %q", tt.wantWorkspace, content)
				}
			}
		})
	}
}

func TestWriteRunAgentsMd(t *testing.T) {
	runDir := t.TempDir()
	runHome := t.TempDir()
	// No AGENTS.md in runHome
	if err := WriteRunAgentsMd(runDir, runHome); err != nil {
		t.Fatalf("WriteRunAgentsMd: %v", err)
	}
	path := filepath.Join(runDir, agentapp.AgentsMdFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written AGENTS.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "## Run directory layout") {
		t.Errorf("written file missing structure: %q", content)
	}
	if !strings.Contains(content, "**home/**") {
		t.Errorf("written file missing home/ doc: %q", content)
	}
}
