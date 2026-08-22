package job

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/infra/proc"
)

// commandSpec runs command through the platform shell, as the Bash tool's
// background path will after sandbox resolution.
func commandSpec(command string) CommandSpec {
	if runtime.GOOS == "windows" {
		return CommandSpec{Command: command, Name: "cmd", Args: []string{"/c", command}}
	}
	return CommandSpec{Command: command, Name: "sh", Args: []string{"-c", command}}
}

func sleepCommand() string {
	if runtime.GOOS == "windows" {
		return "ping -n 31 127.0.0.1 > NUL"
	}
	return "sleep 30"
}

func closeManager(t *testing.T, m *Manager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := m.Close(ctx); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func waitTerminal(t *testing.T, events <-chan Event) Job {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case e := <-events:
			if !e.Job.Running() {
				return e.Job
			}
		case <-deadline:
			t.Fatal("no terminal event")
		}
	}
}

func TestCommandJobLifecycle(t *testing.T) {
	m := NewManager()
	defer closeManager(t, m)
	events, cancel := m.Subscribe("")
	defer cancel()

	j, err := m.StartCommand(commandSpec("echo done"), Provenance{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if j.ID == "" || !strings.HasPrefix(j.ID, "jb_") {
		t.Fatalf("job ID = %q", j.ID)
	}
	if j.State != StateRunning || j.Kind != KindCommand {
		t.Fatalf("job = %+v", j)
	}

	final := waitTerminal(t, events)
	if final.State != StateSucceeded || final.ID != j.ID {
		t.Fatalf("final = %+v", final)
	}
	if final.EndedAt.IsZero() {
		t.Fatal("EndedAt not set")
	}

	chunk, err := m.Output(j.ID, proc.Stdout, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(chunk.Data)); got != "done" {
		t.Fatalf("output = %q", got)
	}

	got, ok := m.Get(j.ID)
	if !ok || got.State != StateSucceeded {
		t.Fatalf("Get = %+v, %v", got, ok)
	}
}

func TestFailedCommandKeepsExitCode(t *testing.T) {
	m := NewManager()
	defer closeManager(t, m)
	events, cancel := m.Subscribe("")
	defer cancel()

	if _, err := m.StartCommand(commandSpec("exit 7"), Provenance{}); err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, events)
	if final.State != StateFailed || final.ExitCode != 7 {
		t.Fatalf("final = %+v", final)
	}
}

func TestStopBecomesCanceledUserStop(t *testing.T) {
	m := NewManager()
	defer closeManager(t, m)
	events, cancel := m.Subscribe("")
	defer cancel()

	spec := commandSpec(sleepCommand())
	j, err := m.StartCommand(spec, Provenance{})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(j.ID); err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, events)
	if final.State != StateCanceled || final.StopReason != StopUser {
		t.Fatalf("final = %+v", final)
	}
}

func TestTimeoutBecomesCanceledTimeout(t *testing.T) {
	m := NewManager()
	defer closeManager(t, m)
	events, cancel := m.Subscribe("")
	defer cancel()

	spec := commandSpec(sleepCommand())
	spec.Timeout = 200 * time.Millisecond
	if _, err := m.StartCommand(spec, Provenance{}); err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, events)
	if final.State != StateCanceled || final.StopReason != StopTimeout {
		t.Fatalf("final = %+v", final)
	}
}

func TestConcurrencyLimitRefuses(t *testing.T) {
	m := NewManager()
	m.maxCommands = 1
	defer closeManager(t, m)

	j, err := m.StartCommand(commandSpec(sleepCommand()), Provenance{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.StartCommand(commandSpec("echo hi"), Provenance{}); err == nil {
		t.Fatal("expected limit refusal")
	}
	if err := m.Stop(j.ID); err != nil {
		t.Fatal(err)
	}
	<-time.After(10 * time.Millisecond)
	// After the slot frees, starting works again.
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err = m.StartCommand(commandSpec("echo hi"), Provenance{}); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("limit never released: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCloseSweepsRunningJobs(t *testing.T) {
	m := NewManager()
	events, cancel := m.Subscribe("")
	defer cancel()

	j, err := m.StartCommand(commandSpec(sleepCommand()), Provenance{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelCtx := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelCtx()
	if err := m.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	final := waitTerminal(t, events)
	if final.ID != j.ID || final.State != StateCanceled || final.StopReason != StopShutdown {
		t.Fatalf("final = %+v", final)
	}
	// A closed manager refuses new work.
	if _, err := m.StartCommand(commandSpec("echo hi"), Provenance{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("start after close = %v, want ErrClosed", err)
	}
}

func TestSubscribeFiltersBySession(t *testing.T) {
	m := NewManager()
	defer closeManager(t, m)
	mine, cancelMine := m.Subscribe("s1")
	defer cancelMine()
	other, cancelOther := m.Subscribe("s2")
	defer cancelOther()

	if _, err := m.StartCommand(commandSpec("echo hi"), Provenance{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, mine)
	if final.Provenance.SessionID != "s1" {
		t.Fatalf("final = %+v", final)
	}
	select {
	case e := <-other:
		t.Fatalf("s2 subscriber saw %+v", e.Job)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSubagentJobLifecycle(t *testing.T) {
	m := NewManager()
	defer closeManager(t, m)
	events, cancel := m.Subscribe("")
	defer cancel()

	j, err := m.StartSubagent("investigate flaky test", 0, Provenance{SessionID: "s1", ParentTraceID: "rt_p", ParentToolCallID: "call_t"},
		func(ctx context.Context) (string, error) { return "the answer", nil })
	if err != nil {
		t.Fatal(err)
	}
	if j.Kind != KindSubagent || j.State != StateRunning || j.Command != "investigate flaky test" {
		t.Fatalf("job = %+v", j)
	}
	final := waitTerminal(t, events)
	if final.State != StateSucceeded {
		t.Fatalf("final = %+v", final)
	}
	if final.Provenance.ParentTraceID != "rt_p" || final.Provenance.ParentToolCallID != "call_t" {
		t.Fatalf("provenance = %+v", final.Provenance)
	}
	chunk, err := m.Output(j.ID, proc.Stdout, 0, 0)
	if err != nil || string(chunk.Data) != "the answer" {
		t.Fatalf("Output = %q, %v", chunk.Data, err)
	}
	// Cursor semantics hold for reply reads too.
	chunk, _ = m.Output(j.ID, proc.Stdout, 4, 0)
	if string(chunk.Data) != "answer" || chunk.Next != 10 {
		t.Fatalf("chunk = %+v", chunk)
	}
}

func TestSubagentJobFailure(t *testing.T) {
	m := NewManager()
	defer closeManager(t, m)
	events, cancel := m.Subscribe("")
	defer cancel()

	if _, err := m.StartSubagent("doomed", 0, Provenance{},
		func(ctx context.Context) (string, error) { return "", errors.New("model unavailable") }); err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, events)
	if final.State != StateFailed || final.Err != "model unavailable" {
		t.Fatalf("final = %+v", final)
	}
}

func TestSubagentJobStopCancelsContext(t *testing.T) {
	m := NewManager()
	defer closeManager(t, m)
	events, cancel := m.Subscribe("")
	defer cancel()

	started := make(chan struct{})
	j, err := m.StartSubagent("long delegation", 0, Provenance{},
		func(ctx context.Context) (string, error) {
			close(started)
			<-ctx.Done()
			return "", ctx.Err()
		})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := m.Stop(j.ID); err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, events)
	if final.State != StateCanceled || final.StopReason != StopUser {
		t.Fatalf("final = %+v", final)
	}
}

func TestSubagentJobLimit(t *testing.T) {
	m := NewManager()
	m.maxSubagents = 1
	defer closeManager(t, m)

	block := make(chan struct{})
	j, err := m.StartSubagent("first", 0, Provenance{}, func(ctx context.Context) (string, error) {
		select {
		case <-block:
		case <-ctx.Done():
		}
		return "done", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.StartSubagent("second", 0, Provenance{}, func(ctx context.Context) (string, error) { return "", nil }); err == nil {
		t.Fatal("expected limit refusal")
	}
	close(block)
	_ = j
}

// Close releases subscribers so a consumer ranging over the channel exits,
// after buffered terminal events are still readable.
func TestCloseReleasesSubscribers(t *testing.T) {
	m := NewManager()
	events, cancel := m.Subscribe("")
	defer cancel() // idempotent with the manager's own shut

	ctx, cancelCtx := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelCtx()
	if err := m.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("unexpected buffered event on an idle manager")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subscriber channel not closed by manager Close")
	}
}

func TestOutputUnknownJob(t *testing.T) {
	m := NewManager()
	defer closeManager(t, m)
	if _, err := m.Output("jb_missing", proc.Stdout, 0, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := m.Stop("jb_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
