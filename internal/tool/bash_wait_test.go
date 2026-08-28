package tool

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/util"
)

// A command that starts a background process and exits leaves that process
// holding the write end of the pipe the tool reads its output from. os/exec's
// Wait blocks until every write end closes, so without a bound the tool waits
// for the *background* process, not for the command — and a killed command
// waits forever, because killing the shell does not close what a grandchild
// inherited.
//
// This was not theoretical. A benchmark trial sat on one Bash call for two
// hours under a documented 120-second timeout: the agent had started a server
// earlier in the task, and every later command inherited its pipe.
func TestABackgroundProcessDoesNotHoldTheToolOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell spawning syntax below is POSIX")
	}
	// Short, so the test observes the mechanism rather than spending the real
	// grace period. The production value is what ships; this is the same code
	// path either way.
	restore := waitDelay
	waitDelay = 200 * time.Millisecond
	t.Cleanup(func() { waitDelay = restore })

	bash := NewBash(util.FixedRoot(testWorkspace(t, "")))
	// The command finishes at once; the sleep it started holds the pipe far
	// longer than the tool may wait for it.
	args := map[string]any{"command": "sleep 60 & echo started"}

	done := make(chan string, 1)
	go func() {
		out, err := bash.Execute(context.Background(), args)
		if err != nil {
			done <- "error: " + err.Error()
			return
		}
		done <- out
	}()

	select {
	case out := <-done:
		if !strings.Contains(out, "started") {
			t.Errorf("output = %q, want what the command printed before the tool stopped listening", out)
		}
		// The model is told the output may be short rather than being handed a
		// bare failure for a command that worked.
		if !strings.Contains(out, "left a process holding its output") {
			t.Errorf("output = %q, want the lingering process named", out)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the tool is still waiting on a background process the command left behind")
	}
}
