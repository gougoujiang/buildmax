package proc

import (
	"bytes"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// shellSpec runs command through the platform shell, like a background Bash
// call would after sandbox resolution.
func shellSpec(command string) Spec {
	if runtime.GOOS == "windows" {
		return Spec{Name: "cmd", Args: []string{"/c", command}}
	}
	return Spec{Name: "sh", Args: []string{"-c", command}}
}

func waitDone(t *testing.T, p *Proc, timeout time.Duration) Result {
	t.Helper()
	select {
	case <-p.Done():
	case <-time.After(timeout):
		p.Stop()
		t.Fatal("process did not finish in time")
	}
	res, ok := p.Result()
	if !ok {
		t.Fatal("Done closed but Result not ready")
	}
	return res
}

func TestCapturesOutputAndExitCode(t *testing.T) {
	p, err := Start(shellSpec("echo hi"))
	if err != nil {
		t.Fatal(err)
	}
	res := waitDone(t, p, 10*time.Second)
	if res.Reason != ReasonExited || res.ExitCode != 0 || res.Err != nil {
		t.Fatalf("unexpected result: %+v", res)
	}
	out := p.Output(Stdout, 0, 0)
	if got := strings.TrimSpace(string(out.Data)); got != "hi" {
		t.Fatalf("stdout = %q", got)
	}
	if out.Dropped != 0 {
		t.Fatalf("dropped = %d", out.Dropped)
	}
}

func TestNonZeroExit(t *testing.T) {
	p, err := Start(shellSpec("exit 3"))
	if err != nil {
		t.Fatal(err)
	}
	res := waitDone(t, p, 10*time.Second)
	if res.Reason != ReasonExited || res.ExitCode != 3 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestStderrIsSeparate(t *testing.T) {
	p, err := Start(shellSpec("echo oops 1>&2"))
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, p, 10*time.Second)
	if got := strings.TrimSpace(string(p.Output(Stderr, 0, 0).Data)); got != "oops" {
		t.Fatalf("stderr = %q", got)
	}
	if got := bytes.TrimSpace(p.Output(Stdout, 0, 0).Data); len(got) != 0 {
		t.Fatalf("stdout = %q", got)
	}
}

func TestSpawnFailure(t *testing.T) {
	if _, err := Start(Spec{Name: "definitely-not-a-real-binary-xyz"}); err == nil {
		t.Fatal("expected spawn error")
	}
	if _, err := Start(Spec{}); err == nil {
		t.Fatal("expected empty-name error")
	}
}

func sleepCommand(seconds int) string {
	if runtime.GOOS == "windows" {
		return "ping -n " + strconv.Itoa(seconds+1) + " 127.0.0.1 > NUL"
	}
	return "sleep " + strconv.Itoa(seconds)
}

func TestTimeout(t *testing.T) {
	spec := shellSpec(sleepCommand(30))
	spec.Timeout = 200 * time.Millisecond
	spec.StopGrace = time.Second
	p, err := Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	res := waitDone(t, p, 10*time.Second)
	if res.Reason != ReasonTimeout {
		t.Fatalf("reason = %q, want timeout", res.Reason)
	}
}

func TestStop(t *testing.T) {
	spec := shellSpec(sleepCommand(30))
	spec.StopGrace = time.Second
	p, err := Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	p.Stop()
	p.Stop() // idempotent
	res := waitDone(t, p, 10*time.Second)
	if res.Reason != ReasonStopped {
		t.Fatalf("reason = %q, want stopped", res.Reason)
	}
}

func TestOutputCursorAndDrop(t *testing.T) {
	r := newRing(8)
	if _, err := r.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	chunk := r.read(0, 2)
	if string(chunk.Data) != "ab" || chunk.Next != 2 || chunk.Dropped != 0 {
		t.Fatalf("chunk = %+v", chunk)
	}
	if _, err := r.Write([]byte("efghij")); err != nil { // total 10 > 8: drops "ab"
		t.Fatal(err)
	}
	chunk = r.read(chunk.Next, 0)
	if string(chunk.Data) != "cdefghij" || chunk.Dropped != 0 {
		t.Fatalf("chunk = %+v", chunk)
	}
	chunk = r.read(0, 0)
	if string(chunk.Data) != "cdefghij" || chunk.Dropped != 2 {
		t.Fatalf("chunk = %+v", chunk)
	}
	// A cursor past the end stays put instead of inventing data.
	chunk = r.read(100, 0)
	if len(chunk.Data) != 0 || chunk.Next != 10 {
		t.Fatalf("chunk = %+v", chunk)
	}
}

func TestRingOversizedWrite(t *testing.T) {
	r := newRing(4)
	if _, err := r.Write([]byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	chunk := r.read(0, 0)
	if string(chunk.Data) != "6789" || chunk.Dropped != 6 {
		t.Fatalf("chunk = %+v", chunk)
	}
}

func TestRingDropIsUTF8Safe(t *testing.T) {
	r := newRing(5)
	if _, err := r.Write([]byte("a世界")); err != nil { // 1 + 3 + 3 bytes; keeps last 5
		t.Fatal(err)
	}
	chunk := r.read(0, 0)
	if string(chunk.Data) != "界" {
		t.Fatalf("data = %q", chunk.Data)
	}
	// 2 dropped by the ring plus 2 skipped continuation bytes.
	if chunk.Dropped != 4 {
		t.Fatalf("dropped = %d", chunk.Dropped)
	}
}
