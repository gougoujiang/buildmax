//go:build !windows

package clie2e

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

// The approval prompt is the one outcome print mode cannot reach: a surface
// with no approval handler turns an Ask into a Deny, so answering one takes a
// terminal. The session lock below needs a terminal for a different reason: it
// needs a process that keeps a session open, and the TUI is the only surface
// that does. Everything else stays in print mode, because a terminal-driven
// test costs more to run and more to diagnose than the paths beside it.

func TestApprovingAtTheTerminalLetsTheWriteThrough(t *testing.T) {
	server := startModel(t, "write-a-file.json")
	workspace := t.TempDir()
	session := startTUI(t, writeHome(t, server, map[string]string{"Write": "ask"}), workspace)

	session.waitFor(t, "ctrl+c: quit")
	session.send(t, "write notes.txt\r")
	session.waitFor(t, "Tool: Write")
	session.send(t, "y")
	session.waitFor(t, "wrote notes.txt")

	written, err := os.ReadFile(filepath.Join(workspace, "notes.txt"))
	if err != nil {
		t.Fatalf("an approved write did not reach the workspace: %v\n%s", err, session.text())
	}
	if string(written) != "scripted content\n" {
		t.Fatalf("file content = %q, want the scripted content", written)
	}
	if remaining := server.Remaining(); remaining != 0 {
		t.Fatalf("unconsumed scenario steps = %d, want 0", remaining)
	}
}

func TestDenyingAtTheTerminalStopsTheWrite(t *testing.T) {
	server := startModel(t, "write-a-file.json")
	workspace := t.TempDir()
	session := startTUI(t, writeHome(t, server, map[string]string{"Write": "ask"}), workspace)

	session.waitFor(t, "ctrl+c: quit")
	session.send(t, "write notes.txt\r")
	session.waitFor(t, "Tool: Write")
	session.send(t, "n")
	session.waitFor(t, "denied")

	if _, err := os.Stat(filepath.Join(workspace, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("a denied write must not touch the workspace (stat err = %v)\n%s", err, session.text())
	}
	// The turn after a denial still happens: the model is told, and answers.
	session.waitFor(t, "wrote notes.txt")
	if remaining := server.Remaining(); remaining != 0 {
		t.Fatalf("unconsumed scenario steps = %d, want 0", remaining)
	}
}

// TestASessionOpenInOneProcessIsRefusedToAnother is the cross-process half of
// the writer lock. The store's own tests take the lock twice inside one
// process, which proves the bookkeeping; what a person is promised is that a
// session open in one window cannot be opened in another, and flock and
// LockFileEx only mean that between real processes. This is also why the test
// lives here rather than beside the store.
func TestASessionOpenInOneProcessIsRefusedToAnother(t *testing.T) {
	server := startModel(t, "answer-once.json")
	workspace := t.TempDir()
	home := writeHome(t, server, nil)

	created := run(t, home, workspace, "-p", "make me a session", "--output", "jsonl")
	sessionID := created.field("result", "session_id")
	if sessionID == "" {
		t.Fatalf("the first run reported no session id\nstdout:\n%s", created.stdout)
	}

	// The TUI holds the session for as long as it is up, which is what gives a
	// second process something to collide with.
	holder := startTUI(t, home, workspace, "-r", sessionID)
	holder.waitFor(t, "ctrl+c: quit")

	blocked := run(t, home, workspace, "-r", sessionID, "-p", "and again")

	if blocked.exitCode == 0 {
		t.Fatalf("the second process opened a session the TUI holds\nstdout:\n%s", blocked.stdout)
	}
	if !strings.Contains(blocked.stderr, "open in another process") {
		t.Fatalf("the refusal does not say the session is held elsewhere:\n%s", blocked.stderr)
	}
	// Refused before the model, not after: a run that asked and then failed
	// would have spent a turn and possibly written something.
	if remaining := server.Remaining(); remaining != 0 {
		t.Fatalf("unconsumed scenario steps = %d, want 0 — the refused run reached the model", remaining)
	}
}

// ptySession is the TUI running on a pseudo-terminal, with everything it has
// drawn so far.
type ptySession struct {
	pty *os.File
	mu  sync.Mutex
	buf []byte
}

func startTUI(t *testing.T, home, workspace string, args ...string) *ptySession {
	t.Helper()
	cmd := exec.Command(binary, append(args, "--workspace", workspace)...)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "BUILDMAX_HOME="+home, "TERM=xterm-256color")
	// A wide terminal keeps the strings these tests match on from being wrapped
	// mid-word, which is the flake this kind of test is famous for.
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 50, Cols: 200})
	if err != nil {
		t.Fatalf("start the TUI on a pty: %v", err)
	}
	session := &ptySession{pty: ptmx}
	go session.read()

	t.Cleanup(func() {
		_, _ = ptmx.Write([]byte{0x03}) // ctrl+c: quit
		done := make(chan struct{})
		go func() { _, _ = cmd.Process.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
		_ = ptmx.Close()
	})
	return session
}

func (s *ptySession) read() {
	chunk := make([]byte, 4096)
	for {
		n, err := s.pty.Read(chunk)
		if n > 0 {
			s.mu.Lock()
			s.buf = append(s.buf, chunk[:n]...)
			s.mu.Unlock()
			s.answerTerminalQueries(chunk[:n])
		}
		if err != nil {
			return
		}
	}
}

// Terminal capability queries the TUI writes on startup, and the answers a
// terminal emulator would send back.
//
// Without these it draws nothing until its own timeout expires, which costs
// five seconds a test and looks like the agent being slow. A pty has no
// emulator behind it, so the harness plays that part.
var terminalAnswers = []struct{ query, answer string }{
	{"\x1b]11;?", "\x1b]11;rgb:0000/0000/0000\x1b\\"}, // background colour
	{"\x1b[6n", "\x1b[1;1R"},                          // cursor position
}

func (s *ptySession) answerTerminalQueries(out []byte) {
	for _, exchange := range terminalAnswers {
		if bytes.Contains(out, []byte(exchange.query)) {
			_, _ = io.WriteString(s.pty, exchange.answer)
		}
	}
}

// text is everything drawn so far with the escape sequences removed, which is
// what a person would have read off the screen.
func (s *ptySession) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return stripANSI(string(s.buf))
}

func (s *ptySession) send(t *testing.T, keys string) {
	t.Helper()
	if _, err := io.WriteString(s.pty, keys); err != nil {
		t.Fatalf("send %q to the TUI: %v", keys, err)
	}
}

// waitFor blocks until the terminal has shown want. The failure prints the
// whole screen: a terminal test that only says "timed out" is unfixable.
func (s *ptySession) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(s.text(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the TUI never showed %q within 20s. What it drew:\n%s", want, s.text())
}

var ansiPattern = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]|\x1b\[[0-9;?]*[ -/]*[@-~]`)

func stripANSI(s string) string { return ansiPattern.ReplaceAllString(s, "") }
