package cli

import (
	"strings"
	"testing"
	"time"

	cllm "github.com/icloudbb/buildmax/internal/core/llm"
	"github.com/icloudbb/buildmax/internal/core/session"
)

func renderUsage(r session.UsageReport) string {
	var b strings.Builder
	writeUsage(&b, r)
	return b.String()
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want time.Time
	}{
		{"72h", now.Add(-72 * time.Hour)},
		{"7d", now.AddDate(0, 0, -7)},
		{"2w", now.AddDate(0, 0, -14)},
		{"2026-09-01", time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)},
	}
	for _, c := range cases {
		got, err := parseSince(c.in, now)
		if err != nil {
			t.Errorf("parseSince(%q) error: %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("parseSince(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := parseSince("yesterday", now); err == nil {
		t.Error("parseSince accepted an unparseable window")
	}
}

func TestWriteUsage_EmptyRangeSaysSo(t *testing.T) {
	out := renderUsage(session.UsageReport{GroupBy: session.GroupByDay})
	if !strings.Contains(out, "no sessions in range") {
		t.Errorf("empty report does not say so:\n%s", out)
	}
}

func TestWriteUsage_ReportsTotalAndPartialCaveat(t *testing.T) {
	r := session.UsageReport{
		GroupBy: session.GroupByModel,
		Buckets: []session.UsageBucket{
			{Key: "opus", Sessions: 2, PromptTokens: 1500, CompletionTokens: 200, Cost: &cllm.Cost{Currency: "USD", Total: 500}},
		},
		Total: session.UsageBucket{Key: "total", Sessions: 2, PromptTokens: 1500, CompletionTokens: 200, Cost: &cllm.Cost{Currency: "USD", Total: 500}, CostIncomplete: true},
	}
	out := renderUsage(r)
	if !strings.Contains(out, "Usage by model") {
		t.Errorf("missing heading:\n%s", out)
	}
	if !strings.Contains(out, "opus") || !strings.Contains(out, "1,500") {
		t.Errorf("missing bucket row:\n%s", out)
	}
	if !strings.Contains(out, "TOTAL") {
		t.Errorf("missing total row:\n%s", out)
	}
	if !strings.Contains(out, "understates") {
		t.Errorf("incomplete total did not print the caveat:\n%s", out)
	}
}

// An unpriced bucket says so rather than showing a zero cost that reads as free.
func TestWriteUsage_UnpricedSaysNotPriced(t *testing.T) {
	r := session.UsageReport{
		GroupBy: session.GroupByDay,
		Buckets: []session.UsageBucket{{Key: "2026-09-01", Sessions: 1, PromptTokens: 10}},
		Total:   session.UsageBucket{Key: "total", Sessions: 1, PromptTokens: 10},
	}
	out := renderUsage(r)
	if !strings.Contains(out, "not priced") {
		t.Errorf("unpriced bucket did not say so:\n%s", out)
	}
}

func TestUsageKeyLabel_ShortensWorkspacePath(t *testing.T) {
	if got := usageKeyLabel("/home/x/my-proj", session.GroupByWorkspace); got != "my-proj" {
		t.Errorf("workspace label = %q, want my-proj", got)
	}
	if got := usageKeyLabel("(none)", session.GroupByWorkspace); got != "(none)" {
		t.Errorf("(none) label = %q, want unchanged", got)
	}
	if got := usageKeyLabel("2026-09-01", session.GroupByDay); got != "2026-09-01" {
		t.Errorf("day label = %q, want unchanged", got)
	}
}
