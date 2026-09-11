package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"

	"github.com/spf13/cobra"
)

func newUsageCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "usage",
		Short: "Show what recent sessions spent, grouped by day, workspace, or model",
		Long: `Sum token and cost totals across every local session, so a week of use
answers as one figure instead of one session at a time.

The numbers come from the session index projection, not from re-reading each
session, so the report costs one file read no matter how many sessions it
covers. Tokens and cost are each session's own running totals, accumulated at
the rates in force when they ran; nothing is repriced on read. A session no
model priced still counts its tokens, and the report says the money understates
it rather than dropping it.

Group by day to see when the spend happened, by workspace to see where, or by
model to see which model it went to. --since narrows the report to sessions
started within a window: a Go duration (72h), a day or week count (7d, 2w), or a
date (2026-09-01).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			groupStr, _ := cmd.Flags().GetString("group-by")
			group, ok := session.ParseUsageGroup(groupStr)
			if !ok {
				err := fmt.Errorf("invalid --group-by %q: want day, workspace, or model", groupStr)
				fmt.Fprintln(os.Stderr, err.Error())
				return &ExitError{Code: ExitUsage, Err: err}
			}
			var since *time.Time
			if s, _ := cmd.Flags().GetString("since"); s != "" {
				t, err := parseSince(s, time.Now())
				if err != nil {
					fmt.Fprintln(os.Stderr, err.Error())
					return &ExitError{Code: ExitUsage, Err: err}
				}
				since = &t
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			return runUsage(cmd.Context(), os.Stdout, group, since, asJSON)
		},
	}
	c.Flags().String("group-by", "day", "group sessions by day, workspace, or model")
	c.Flags().String("since", "", "only sessions started within a window: a duration (72h), day/week count (7d, 2w), or date (2026-09-01)")
	c.Flags().Bool("json", false, "emit the report as JSON instead of a table")
	return c
}

func runUsage(_ context.Context, w io.Writer, group session.UsageGroup, since *time.Time, asJSON bool) error {
	rows, err := agentapp.NewSessionManager(config.SessionsDir()).List()
	if err != nil {
		return fmt.Errorf("load session list: %w", err)
	}
	report := session.AggregateUsage(rows, group, since, time.Local)
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	writeUsage(w, report)
	return nil
}

// parseSince resolves a window expression to the earliest start time a session
// may have. It accepts a Go duration, a day or week count (7d, 2w) — units Go's
// own parser omits — and a plain date, whose start is the local midnight of
// that day so "--since 2026-09-01" includes everything from the first onward.
func parseSince(s string, now time.Time) (time.Time, error) {
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, nil
	}
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(n)
		if err == nil && days >= 0 {
			return now.AddDate(0, 0, -days), nil
		}
	}
	if n, ok := strings.CutSuffix(s, "w"); ok {
		weeks, err := strconv.Atoi(n)
		if err == nil && weeks >= 0 {
			return now.AddDate(0, 0, -7*weeks), nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil && d >= 0 {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("invalid --since %q: want a duration (72h), a day/week count (7d, 2w), or a date (2006-01-02)", s)
}

func writeUsage(w io.Writer, r session.UsageReport) {
	heading := map[session.UsageGroup]string{
		session.GroupByDay:       "Usage by day",
		session.GroupByWorkspace: "Usage by workspace",
		session.GroupByModel:     "Usage by model",
	}[r.GroupBy]
	fmt.Fprintln(w, heading)
	if r.Since != nil {
		fmt.Fprintf(w, "since %s\n", r.Since.Local().Format("2006-01-02 15:04"))
	}
	if r.Total.Sessions == 0 {
		fmt.Fprintln(w, "  no sessions in range")
		return
	}

	label := map[session.UsageGroup]string{
		session.GroupByDay:       "DAY",
		session.GroupByWorkspace: "WORKSPACE",
		session.GroupByModel:     "MODEL",
	}[r.GroupBy]

	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  %s\tSESSIONS\tTOKENS (in/out)\tCOST\n", label)
	for _, b := range r.Buckets {
		fmt.Fprintf(tw, "  %s\t%d\t%s / %s\t%s\n",
			usageKeyLabel(b.Key, r.GroupBy), b.Sessions,
			formatCount(b.PromptTokens), formatCount(b.CompletionTokens), usageCost(b))
	}
	fmt.Fprintf(tw, "  %s\t%d\t%s / %s\t%s\n",
		"TOTAL", r.Total.Sessions,
		formatCount(r.Total.PromptTokens), formatCount(r.Total.CompletionTokens), usageCost(r.Total))
	_ = tw.Flush()

	if r.Total.CostIncomplete {
		fmt.Fprintln(w, "\n! Some sessions ran against an unpriced model or a different currency, so the cost understates the real spend.")
	}
}

// usageKeyLabel shortens a workspace path to its trailing component, since the
// full path repeated down a column is noise and the leaf is what tells two
// workspaces apart at a glance. Day and model keys are already short.
func usageKeyLabel(key string, group session.UsageGroup) string {
	if group != session.GroupByWorkspace || key == "(none)" {
		return key
	}
	trimmed := strings.TrimRight(key, "/")
	if i := strings.LastIndexByte(trimmed, '/'); i >= 0 && i < len(trimmed)-1 {
		return trimmed[i+1:]
	}
	return key
}

func usageCost(b session.UsageBucket) string {
	if b.Cost == nil {
		return "not priced"
	}
	s := cllm.FormatAmount(b.Cost.Total) + " " + b.Cost.Currency
	if b.CostIncomplete {
		s += " (partial)"
	}
	return s
}
