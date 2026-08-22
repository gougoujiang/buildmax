package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"

	tea "charm.land/bubbletea/v2"
)

const slashJobsInlinePanelMaxLines = 12

// jobEventMsg carries one background-job lifecycle event into the update loop.
type jobEventMsg struct {
	Event job.Event
}

// listenJobsCmd waits for the next job event. It re-arms itself from the
// jobEventMsg handler, one event per Cmd, the same shape as the stream channel.
func listenJobsCmd(events <-chan job.Event) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		e, ok := <-events
		if !ok {
			return nil
		}
		return jobEventMsg{Event: e}
	}
}

// handleJobEvent prints terminal transitions to the scrollback and re-arms the
// listener. Job starts are already visible as the Bash tool's result, so only
// the ending is news.
func handleJobEvent(m *Model, msg jobEventMsg) (tea.Model, tea.Cmd) {
	listen := listenJobsCmd(m.jobEvents)
	if msg.Event.Job.Running() {
		return m, listen
	}
	line := formatJobEventForScrollback(msg.Event.Job)
	return m, tea.Batch(tea.Println(line+"\n"), listen)
}

// formatJobEventForScrollback renders one finished job as a dim notice line:
// visible without pretending to be part of the conversation.
func formatJobEventForScrollback(j job.Job) string {
	glyph := "✓"
	if j.State != job.StateSucceeded {
		glyph = "✗"
	}
	detail := string(j.State)
	switch {
	case j.State == job.StateCanceled && j.StopReason != "":
		detail += " (" + string(j.StopReason) + ")"
	case j.State == job.StateFailed && j.Err != "":
		detail += " (" + j.Err + ")"
	}
	line := fmt.Sprintf("%s job %s %s after %s — %s", glyph, j.ID, detail, jobAge(j), jobCommandSummary(j.Command, 60))
	return queuedMessageStyle.Render(line)
}

// slashJobsPanel implements slashPanel for the /tasks overlay. It reads live
// state from the manager on every render, so an open panel tracks job
// progress without its own refresh plumbing.
type slashJobsPanel struct {
	selected int
}

func openSlashJobs(m *Model) (tea.Model, tea.Cmd) {
	if m.opts.App == nil || m.opts.App.Jobs() == nil {
		m.err = "background jobs are not available on this surface"
		return m, nil
	}
	return m.openPanel(&slashJobsPanel{})
}

func (p *slashJobsPanel) jobs(m *Model) []job.Job {
	if m.opts.App == nil || m.opts.App.Jobs() == nil {
		return nil
	}
	return m.opts.App.Jobs().List()
}

func (p *slashJobsPanel) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	jobs := p.jobs(m)
	switch msg.Code {
	case tea.KeyEscape:
		_, cmd := m.closeActivePanel()
		return true, cmd
	case tea.KeyUp:
		if p.selected > 0 {
			p.selected--
		}
		return true, nil
	case tea.KeyDown:
		if p.selected < len(jobs)-1 {
			p.selected++
		}
		return true, nil
	}
	if msg.String() == "s" {
		if p.selected < len(jobs) && jobs[p.selected].Running() {
			_ = m.opts.App.Jobs().Stop(jobs[p.selected].ID)
		}
		return true, nil
	}
	return false, nil
}

func (p *slashJobsPanel) FooterHint() string {
	return "↑/↓: select · s: stop job · esc: close"
}

func (p *slashJobsPanel) OnClose(_ *Model) {}

func (p *slashJobsPanel) Render(m *Model, maxLineWidth int) string {
	jobs := p.jobs(m)
	if p.selected >= len(jobs) && len(jobs) > 0 {
		p.selected = len(jobs) - 1
	}
	var b strings.Builder
	b.WriteString(slashPanelTitleStyle.Render("Background Jobs"))
	b.WriteString("\n\n")
	if len(jobs) == 0 {
		b.WriteString("No background jobs. Ask for a command with run_in_background to start one.")
		return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
	}
	shown := 0
	for i, j := range jobs {
		if shown >= slashJobsInlinePanelMaxLines {
			fmt.Fprintf(&b, "… %d more\n", len(jobs)-shown)
			break
		}
		marker := "  "
		if i == p.selected {
			marker = "> "
		}
		state := string(j.State)
		if j.State == job.StateCanceled && j.StopReason != "" {
			state += " (" + string(j.StopReason) + ")"
		}
		line := fmt.Sprintf("%s%s  %-9s  %6s  %s", marker, j.ID, state, jobAge(j), jobCommandSummary(j.Command, 40))
		b.WriteString(truncateRunes(line, maxLineWidth))
		b.WriteByte('\n')
		shown++
	}
	b.WriteString("\nJobs share this workspace and stop when the app exits.")
	return strings.TrimRight(b.String(), "\n") + "\n\n↑/↓ select · s stop · esc close"
}

func jobAge(j job.Job) string {
	end := j.EndedAt
	if end.IsZero() {
		end = time.Now()
	}
	return end.Sub(j.CreatedAt).Round(time.Second).String()
}

func jobCommandSummary(cmd string, maxRunes int) string {
	cmd = strings.ReplaceAll(cmd, "\n", " ")
	return truncateRunes(cmd, maxRunes)
}
