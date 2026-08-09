package trace

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gougoujiang/buildmax/internal/core/agent"
)

func TestRecordFromEvent_Mapping(t *testing.T) {
	cases := []struct {
		name     string
		ev       agent.Event
		wantType string
		check    func(t *testing.T, r Record)
	}{
		{"iter", agent.Event{Kind: agent.EventIterStart, Iter: 2}, "iter_start", func(t *testing.T, r Record) {
			if r.Iter != 2 {
				t.Errorf("iter = %d", r.Iter)
			}
		}},
		{"llm_end", agent.Event{Kind: agent.EventLLMEnd, Iter: 1, HasToolCalls: true, PromptTokens: 10, CompletionTokens: 5, Content: "hi"}, "llm_end", func(t *testing.T, r Record) {
			if !r.HasToolCalls || r.PromptTokens != 10 || r.Content != "hi" {
				t.Errorf("bad llm_end: %+v", r)
			}
		}},
		{"tool_end", agent.Event{Kind: agent.EventToolEnd, ToolName: "bash", ToolCallID: "c1", ToolResult: "ok", ToolDuration: 1500 * time.Millisecond}, "tool_end", func(t *testing.T, r Record) {
			if r.Tool != "bash" || r.Result != "ok" || r.DurationMS != 1500 {
				t.Errorf("bad tool_end: %+v", r)
			}
		}},
		{"tool_denied", agent.Event{Kind: agent.EventToolDenied, ToolName: "bash", DenyReason: agent.DenyReasonPolicy}, "tool_denied", func(t *testing.T, r Record) {
			if r.DenyReason != "policy" {
				t.Errorf("deny = %q", r.DenyReason)
			}
		}},
		{"run_end", agent.Event{Kind: agent.EventRunEnd, Stats: agent.RunStats{ToolCalls: 3}, Err: errors.New("boom")}, "run_end", func(t *testing.T, r Record) {
			if r.ToolCalls != 3 || r.Error != "boom" {
				t.Errorf("bad run_end: %+v", r)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, ok := recordFromEvent(c.ev, defaultMaxFieldBytes)
			if !ok {
				t.Fatal("expected ok")
			}
			if r.Type != c.wantType {
				t.Fatalf("type = %q, want %q", r.Type, c.wantType)
			}
			c.check(t, r)
		})
	}
}

func TestRecordFromEvent_DeltaSkipped(t *testing.T) {
	if _, ok := recordFromEvent(agent.Event{Kind: agent.EventLLMDelta, Content: "x"}, defaultMaxFieldBytes); ok {
		t.Error("EventLLMDelta should not be persisted")
	}
}

func TestRecordFromEvent_BoundsAndRedacts(t *testing.T) {
	big := strings.Repeat("a", 100) + " Bearer secrettoken123456"
	r, _ := recordFromEvent(agent.Event{Kind: agent.EventLLMEnd, Content: big}, 20)
	if !strings.Contains(r.Content, "truncated") {
		t.Errorf("expected truncation marker, got %q", r.Content)
	}
	full, _ := recordFromEvent(agent.Event{Kind: agent.EventToolEnd, ToolResult: "x Bearer secrettoken123456"}, defaultMaxFieldBytes)
	if strings.Contains(full.Result, "secrettoken123456") {
		t.Errorf("token not redacted: %q", full.Result)
	}
}

func TestBound(t *testing.T) {
	if got := bound("hello", 0); got != "hello" {
		t.Errorf("max<=0 should not bound: %q", got)
	}
	if got := bound("hello", 100); got != "hello" {
		t.Errorf("under limit should not bound: %q", got)
	}
	got := bound("hello world", 5)
	if !strings.HasPrefix(got, "hello") || !strings.Contains(got, "truncated 6 bytes") {
		t.Errorf("bound = %q", got)
	}
}

// TestBound_RuneBoundary asserts the cut never splits a multi-byte character:
// the kept prefix must stay valid UTF-8 so the JSON encoder does not rewrite
// the tail as U+FFFD.
func TestBound_RuneBoundary(t *testing.T) {
	// Each CJK rune is 3 bytes; a limit of 4 falls inside the second rune.
	got := bound("中文内容", 4)
	if !utf8.ValidString(got) {
		t.Fatalf("bound produced invalid UTF-8: %q", got)
	}
	prefix, _, _ := strings.Cut(got, " … [truncated")
	if prefix != "中" {
		t.Errorf("prefix = %q, want %q", prefix, "中")
	}
	if !strings.Contains(got, "truncated 9 bytes") {
		t.Errorf("bound = %q, want 9 dropped bytes", got)
	}
}
