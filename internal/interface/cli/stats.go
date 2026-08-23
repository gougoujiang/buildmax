package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/util"

	"github.com/spf13/cobra"
)

// maxStatsTools bounds the per-tool table. The list is sorted by weight, so the
// tail is the part nobody reads; a session with sixty tools should still print
// something a person can take in.
const maxStatsTools = 12

func newStatsCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "stats [session-id]",
		Short: "Show what a session spent, what it did, and where its context went",
		Long: `Show one session's statistics.

With no argument, the most recent session by creation time is used.

Tokens and cost come from the session file, which accumulated them turn by turn
at the rates in force for each. Timings, per-tool detail, and the delegated
breakdown come from the session's run traces; where no trace was written, those
lines say so rather than reporting zero.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var id string
			if len(args) == 1 {
				id = args[0]
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			return runStats(os.Stdout, id, asJSON)
		},
	}
	c.Flags().Bool("json", false, "emit the statistics as JSON instead of a table")
	return c
}

func runStats(w io.Writer, id string, asJSON bool) error {
	sessionsDir := config.SessionsDir()
	if id == "" {
		list, err := agentapp.LoadSessionList(sessionsDir)
		if err != nil {
			return fmt.Errorf("load session list: %w", err)
		}
		last := latestSessionItem(list)
		if last == nil {
			return fmt.Errorf("no sessions found; run one with -p PROMPT or start the TUI")
		}
		id = last.ID
	}

	stats, err := agentapp.LoadSessionStats(sessionsDir, config.TracesDir(), id)
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	}
	writeStats(w, stats)
	return nil
}

func writeStats(w io.Writer, s agentapp.SessionStats) {
	title := s.Title
	if title == "" {
		title = "(untitled)"
	}
	fmt.Fprintf(w, "%s\n", title)
	fmt.Fprintf(w, "Session:   %s\n", s.ID)
	if s.Workspace != "" {
		fmt.Fprintf(w, "Workspace: %s\n", s.Workspace)
	}
	fmt.Fprintf(w, "Started:   %s\n", s.CreatedAt.Local().Format(time.RFC3339))
	if len(s.Runs.Models) > 0 {
		fmt.Fprintf(w, "Models:    %s\n", strings.Join(s.Runs.Models, ", "))
	}

	writeStatsSpend(w, s)
	writeStatsContext(w, s)
	writeStatsWork(w, s)
	writeStatsTools(w, s)
	writeStatsCaveats(w, s)
}

func writeStatsSpend(w io.Writer, s agentapp.SessionStats) {
	fmt.Fprintf(w, "\nSpend\n")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Tokens (in/out)\t%s / %s\n",
		formatCount(s.Usage.PromptTokens), formatCount(s.Usage.CompletionTokens))
	// Only where a provider reported cached tokens: "0 / 0" on a provider that
	// reports nothing would claim a miss nobody measured.
	if s.Usage.CacheReadTokens > 0 || s.Usage.CacheWriteTokens > 0 {
		fmt.Fprintf(tw, "  Cache (read/write)\t%s / %s\n",
			formatCount(s.Usage.CacheReadTokens), formatCount(s.Usage.CacheWriteTokens))
	}
	if s.Cost == nil {
		fmt.Fprintf(tw, "  Cost\tnot priced — no model in this session had rates configured\n")
	} else {
		fmt.Fprintf(tw, "  Cost\t%s %s\n", cllm.FormatAmount(s.Cost.Total), s.Cost.Currency)
		fmt.Fprintf(tw, "    input / cache read / cache write / output\t%s / %s / %s / %s\n",
			cllm.FormatAmount(s.Cost.Uncached), cllm.FormatAmount(s.Cost.CacheRead),
			cllm.FormatAmount(s.Cost.CacheWrite), cllm.FormatAmount(s.Cost.Output))
		if saved, ok := s.CacheSaved(); ok {
			fmt.Fprintf(tw, "    saved by caching\t%s of %s uncached\n",
				cllm.FormatAmount(saved), cllm.FormatAmount(s.Cost.Baseline))
		} else if s.Cost.Baseline > 0 {
			fmt.Fprintf(tw, "    saved by caching\tnothing — this session paid more than it would have uncached\n")
		}
	}
	if d := s.Runs.Delegated; d != nil && d.Runs > 0 {
		line := fmt.Sprintf("  Of which delegated\t%d run(s), %s in / %s out",
			d.Runs, formatCount(d.PromptTokens), formatCount(d.CompletionTokens))
		if d.Cost != nil {
			line += fmt.Sprintf(", %s %s", cllm.FormatAmount(d.Cost.Total), d.Cost.Currency)
		}
		fmt.Fprintln(tw, line)
	}
	_ = tw.Flush()
}

func writeStatsContext(w io.Writer, s agentapp.SessionStats) {
	fmt.Fprintf(w, "\nContext\n")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if share, ok := s.ContextPeakShare(); ok {
		fmt.Fprintf(tw, "  Peak window use\t%s of %s (%.0f%%)\n",
			formatCount(s.Runs.PeakContextTokens), formatCount(s.Runs.ContextWindow), share*100)
	} else {
		fmt.Fprintf(tw, "  Peak window use\tnot recorded\n")
	}
	fmt.Fprintf(tw, "  Compactions\t%d", s.Runs.Compactions)
	if s.Conversation.CompactedMessages > 0 {
		fmt.Fprintf(tw, " (%d message(s) summarized away)", s.Conversation.CompactedMessages)
	}
	fmt.Fprintln(tw)
	// The share is only meaningful where a provider reported cache usage at
	// all; a provider that reports nothing has not reported a miss.
	if s.Usage.PromptTokens > 0 && (s.Usage.CacheReadTokens > 0 || s.Usage.CacheWriteTokens > 0) {
		fmt.Fprintf(tw, "  Prompt served from cache\t%.0f%%\n",
			float64(s.Usage.CacheReadTokens)/float64(s.Usage.PromptTokens)*100)
	}
	c := s.Conversation
	fmt.Fprintf(tw, "  History bytes (text / tool output)\t%s / %s\n",
		formatBytes(c.TextBytes), formatBytes(c.ToolResultBytes))
	_ = tw.Flush()
}

func writeStatsWork(w io.Writer, s agentapp.SessionStats) {
	c := s.Conversation
	fmt.Fprintf(w, "\nWork\n")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Your messages\t%d", c.UserMessages)
	if c.BackgroundMessages > 0 {
		fmt.Fprintf(tw, " (plus %d background event(s))", c.BackgroundMessages)
	}
	fmt.Fprintln(tw)
	fmt.Fprintf(tw, "  Assistant turns\t%d\n", c.AssistantTurns)
	fmt.Fprintf(tw, "  Tool calls\t%d\n", c.ToolCalls)
	if s.Runs.ToolFailures > 0 {
		fmt.Fprintf(tw, "  Calls that could not complete\t%d (a command exiting non-zero is not one)\n",
			s.Runs.ToolFailures)
	}
	if s.Runs.ToolDenials > 0 {
		fmt.Fprintf(tw, "  Calls denied\t%d\n", s.Runs.ToolDenials)
	}
	if c.Notes > 0 || c.Todos > 0 {
		fmt.Fprintf(tw, "  Notes / todos\t%d / %d\n", c.Notes, c.Todos)
	}

	if s.Runs.Runs == 0 {
		fmt.Fprintf(tw, "  Runs\tno trace recorded, so timings are unavailable\n")
		_ = tw.Flush()
		return
	}
	fmt.Fprintf(tw, "  Runs\t%d", s.Runs.Runs)
	if s.Runs.Subagents > 0 {
		fmt.Fprintf(tw, " (plus %d subagent run(s))", s.Runs.Subagents)
	}
	fmt.Fprintln(tw)
	fmt.Fprintf(tw, "  Time spent waiting\t%s\n", util.FormatDuration(s.Runs.Wall))
	if model, ok := s.ModelTime(); ok {
		fmt.Fprintf(tw, "    model / tools\t%s / %s\n",
			util.FormatDuration(model), util.FormatDuration(s.Runs.ToolWall))
	} else if s.Runs.ToolWall > 0 {
		// Parallel tool execution can make summed tool time exceed the wall
		// clock. Reporting a negative model time would be worse than saying
		// the split does not divide.
		fmt.Fprintf(tw, "    tools\t%s (overlapping, so it does not subtract from the wall clock)\n",
			util.FormatDuration(s.Runs.ToolWall))
	}
	_ = tw.Flush()
}

func writeStatsTools(w io.Writer, s agentapp.SessionStats) {
	tools := mergeToolStats(s)
	if len(tools) == 0 {
		return
	}
	shown := tools
	if len(shown) > maxStatsTools {
		shown = shown[:maxStatsTools]
	}
	// The note column is dropped when nothing has one, so a clean session does
	// not print a header for an empty column.
	notes := false
	for _, t := range shown {
		if t.Note != "" {
			notes = true
			break
		}
	}

	fmt.Fprintf(w, "\nTools, heaviest first\n")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := "  TOOL\tCALLS\tOUTPUT\tTIME"
	if notes {
		header += "\tNOTE"
	}
	fmt.Fprintln(tw, header)
	for _, t := range shown {
		name := t.Name
		if name == "" {
			name = "(unattributed)"
		}
		output := "-"
		if t.ResultBytes > 0 {
			output = formatBytes(t.ResultBytes)
		}
		spent := "-"
		if t.Wall > 0 {
			spent = util.FormatDuration(t.Wall)
		}
		row := fmt.Sprintf("  %s\t%d\t%s\t%s", name, t.Calls, output, spent)
		if notes {
			row += "\t" + t.Note
		}
		fmt.Fprintln(tw, row)
	}
	_ = tw.Flush()
	if len(tools) > len(shown) {
		fmt.Fprintf(w, "  … and %d more; --json lists them all\n", len(tools)-len(shown))
	}
}

// statsTool is one tool's row, joining what the history knows (how many bytes
// it put back into the context) with what the trace knows (how long it took,
// and how it failed).
type statsTool struct {
	Name        string
	Calls       int
	ResultBytes int
	Wall        time.Duration
	Note        string
}

func mergeToolStats(s agentapp.SessionStats) []statsTool {
	rows := make(map[string]*statsTool)
	row := func(name string) *statsTool {
		r, ok := rows[name]
		if !ok {
			r = &statsTool{Name: name}
			rows[name] = r
		}
		return r
	}
	for _, t := range s.Conversation.Tools {
		r := row(t.Name)
		r.Calls = t.Calls
		r.ResultBytes = t.ResultBytes
	}
	for _, t := range s.Runs.Tools {
		r := row(t.Name)
		// The traces see subagent calls the parent's history never recorded,
		// so a trace count above the history's is the truth about what ran.
		if t.Calls > r.Calls {
			r.Calls = t.Calls
		}
		r.Wall = t.Wall
		var notes []string
		for kind, n := range t.Failures {
			notes = append(notes, fmt.Sprintf("%d %s", n, kind))
		}
		sort.Strings(notes)
		if t.Denials > 0 {
			notes = append(notes, fmt.Sprintf("%d denied", t.Denials))
		}
		r.Note = strings.Join(notes, ", ")
	}
	out := make([]statsTool, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ResultBytes != b.ResultBytes {
			return a.ResultBytes > b.ResultBytes
		}
		if a.Wall != b.Wall {
			return a.Wall > b.Wall
		}
		return a.Name < b.Name
	})
	return out
}

// writeStatsCaveats names what these numbers do not cover. A total that
// silently dropped a killed run is worse than one that says it did.
func writeStatsCaveats(w io.Writer, s agentapp.SessionStats) {
	var lines []string
	if s.CostIncomplete {
		lines = append(lines, "Part of this session ran against an unpriced model or a different currency, so the cost understates it.")
	}
	if s.Runs.Incomplete > 0 {
		lines = append(lines, fmt.Sprintf("%d run(s) ended without writing a trace end record — killed or crashed — so their timings are missing here.", s.Runs.Incomplete))
	}
	if s.Runs.Failed > 0 {
		lines = append(lines, fmt.Sprintf("%d run(s) ended with an error.", s.Runs.Failed))
	}
	if s.Conversation.ToolCalls > 0 && s.Runs.Runs == 0 {
		lines = append(lines, "No run trace was found for this session, so every timing above is unavailable rather than zero.")
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(w)
	for _, l := range lines {
		fmt.Fprintf(w, "! %s\n", l)
	}
}

// formatCount groups a token count so six- and seven-figure numbers stay
// readable at a glance.
func formatCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func formatBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
