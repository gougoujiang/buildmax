package job

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func monitorSpec(command string) MonitorSpec {
	cs := commandSpec(command)
	return MonitorSpec{Command: cs.Command, Name: cs.Name, Args: cs.Args}
}

// requirePOSIXShell skips tests whose watcher commands use sh syntax; the
// pump itself is platform-neutral and covered on Windows by CI's Job Object
// variants.
func requirePOSIXShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("watcher command uses sh syntax")
	}
}

// collectMonitorEvents reads events until the job goes terminal, returning
// delivered lines and the total dropped count.
func collectMonitorEvents(t *testing.T, events <-chan Event) (lines []string, dropped int) {
	t.Helper()
	deadline := time.After(20 * time.Second)
	for {
		select {
		case e := <-events:
			if e.Type == EventMonitorLine {
				dropped += e.DroppedLines
				if e.Line != "" {
					lines = append(lines, e.Line)
				}
				continue
			}
			if !e.Job.Running() {
				return lines, dropped
			}
		case <-deadline:
			t.Fatal("monitor never finished")
		}
	}
}

func TestMonitorDeliversLines(t *testing.T) {
	requirePOSIXShell(t)
	m := NewManager()
	defer closeManager(t, m)
	events, cancel := m.Subscribe("")
	defer cancel()

	j, err := m.StartMonitor(monitorSpec("echo one; echo two"), Provenance{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if j.Kind != KindMonitor || j.Deliver {
		t.Fatalf("job = %+v", j)
	}
	lines, dropped := collectMonitorEvents(t, events)
	if strings.Join(lines, ",") != "one,two" || dropped != 0 {
		t.Fatalf("lines = %v, dropped = %d", lines, dropped)
	}
	final, _ := m.Get(j.ID)
	if final.State != StateSucceeded {
		t.Fatalf("final = %+v", final)
	}
}

func TestMonitorReactSetsDeliver(t *testing.T) {
	m := NewManager()
	defer closeManager(t, m)
	spec := monitorSpec("echo hi")
	spec.React = true
	spec.Persistent = true
	j, err := m.StartMonitor(spec, Provenance{})
	if err != nil {
		t.Fatal(err)
	}
	if !j.Deliver || !j.Persistent {
		t.Fatalf("job = %+v", j)
	}
}

// A flood is bounded: few delivered lines, and every undelivered line
// accounted for in DroppedLines — including a final summary when the monitor
// ends mid-flood.
func TestMonitorBackpressureAccountsForEveryLine(t *testing.T) {
	requirePOSIXShell(t)
	m := NewManager()
	defer closeManager(t, m)
	events, cancel := m.Subscribe("")
	defer cancel()

	const total = 300
	if _, err := m.StartMonitor(monitorSpec("i=0; while [ $i -lt 300 ]; do echo line$i; i=$((i+1)); done"), Provenance{}); err != nil {
		t.Fatal(err)
	}
	lines, dropped := collectMonitorEvents(t, events)
	if len(lines)+dropped != total {
		t.Fatalf("delivered %d + dropped %d != %d", len(lines), dropped, total)
	}
	// The flood lasts well under a second, so deliveries stay near the
	// per-second budget; the bound just needs to prove the flood was cut.
	if len(lines) > 25 {
		t.Fatalf("delivered %d lines, backpressure not applied", len(lines))
	}
}

func TestMonitorLineTruncatedUTF8Safe(t *testing.T) {
	requirePOSIXShell(t)
	m := NewManager()
	defer closeManager(t, m)
	events, cancel := m.Subscribe("")
	defer cancel()

	// 1200 three-byte runes = 3600 bytes, over the 2048 cap.
	if _, err := m.StartMonitor(monitorSpec(`printf '世%.0s' $(seq 1 1200); echo`), Provenance{}); err != nil {
		t.Fatal(err)
	}
	lines, _ := collectMonitorEvents(t, events)
	if len(lines) != 1 {
		t.Fatalf("lines = %d", len(lines))
	}
	line := lines[0]
	if len(line) > maxMonitorLineBytes+len("…") {
		t.Fatalf("line length %d over cap", len(line))
	}
	if !strings.HasSuffix(line, "…") {
		t.Fatalf("truncated line missing marker: %q", line[len(line)-12:])
	}
	for _, r := range strings.TrimSuffix(line, "…") {
		if r != '世' {
			t.Fatalf("mangled rune %q", r)
		}
	}
}

func TestMonitorStopBecomesCanceled(t *testing.T) {
	m := NewManager()
	defer closeManager(t, m)
	events, cancel := m.Subscribe("")
	defer cancel()

	j, err := m.StartMonitor(monitorSpec(sleepCommand()), Provenance{})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(j.ID); err != nil {
		t.Fatal(err)
	}
	_, _ = collectMonitorEvents(t, events)
	final, _ := m.Get(j.ID)
	if final.State != StateCanceled || final.StopReason != StopUser {
		t.Fatalf("final = %+v", final)
	}
}
