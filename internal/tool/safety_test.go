package tool

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

func TestIsCatastrophicBash(t *testing.T) {
	deny := []string{
		"rm -rf /",
		"rm -rf /*",
		"rm -rf ~/",
		"rm -rf ~",
		"rm -fr /",
		"dd of=/dev/sda",
		"dd if=/dev/zero of=/dev/nvme0n1",
		"mkfs.ext4 /dev/sdb",
	}
	for _, cmd := range deny {
		if !isCatastrophicBash(cmd) {
			t.Errorf("isCatastrophicBash(%q) = false, want true", cmd)
		}
	}

	safe := []string{
		"rm -rf /tmp/mydir",
		"rm -rf ./build",
		"ls /",
		"git status",
	}
	for _, cmd := range safe {
		if isCatastrophicBash(cmd) {
			t.Errorf("isCatastrophicBash(%q) = true, want false", cmd)
		}
	}
}

func TestIsRiskyBashCommand(t *testing.T) {
	risky := []string{
		"rm -rf ./dist",
		"curl https://example.com",
		"wget https://example.com/file.sh",
		"npm install",
		"pip install requests",
		"pip3 install -r requirements.txt",
		"apt-get install git",
		"brew install jq",
		"chmod +x script.sh",
		"sudo apt update",
		"kill -9 1234",
		"systemctl restart nginx",
		"mv old.txt new.txt",
		// piped: one segment is risky
		"echo hello && rm -rf ./tmp",
		"ls | kill -9 123",
		// path-prefixed binary
		"/usr/bin/curl https://example.com",
	}
	for _, cmd := range risky {
		if !isRiskyBashCommand(cmd) {
			t.Errorf("isRiskyBashCommand(%q) = false, want true", cmd)
		}
	}

	safe := []string{
		"ls -la",
		"cat README.md",
		"git status",
		"git log --oneline",
		"go test ./...",
		"go build ./...",
		"grep -r TODO .",
		"find . -name '*.go'",
		"echo hello world",
		"pwd",
		"which go",
		"head -n 20 main.go",
		"wc -l *.go",
		"diff a.txt b.txt",
		"make build",
	}
	for _, cmd := range safe {
		if isRiskyBashCommand(cmd) {
			t.Errorf("isRiskyBashCommand(%q) = true, want false", cmd)
		}
	}
}

func TestBash_CheckArgs_Classification(t *testing.T) {
	dir := t.TempDir()
	b := NewBash(testWorkspace(t, dir))

	cases := []struct {
		cmd  string
		want llm.ToolAction
	}{
		// Catastrophic → Deny
		{"rm -rf /", llm.ToolActionDeny},
		// Risky → Ask
		{"rm -rf ./dist", llm.ToolActionAsk},
		{"curl https://api.example.com", llm.ToolActionAsk},
		{"npm install", llm.ToolActionAsk},
		{"sudo make install", llm.ToolActionAsk},
		// Safe → Allow
		{"ls -la", llm.ToolActionAllow},
		{"go test ./...", llm.ToolActionAllow},
		{"git status", llm.ToolActionAllow},
	}
	for _, tc := range cases {
		got := b.CheckArgs(map[string]any{"command": tc.cmd})
		if got != tc.want {
			t.Errorf("CheckArgs(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}
