// Package cli provides the BuildMax CLI commands and interactive Bubble Tea UI.

package cli

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/infra/git"
	"github.com/gougoujiang/buildmax/internal/interface/auth"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

const (
	carouselTick          = 400 // ms between carousel dot updates
	maxStreamPreviewLines = 12  // max lines of streaming buffer shown in live view
)

// toolSpinnerFrames cycles on each carousel tick to show progress while a tool is running.
var toolSpinnerFrames = [3]string{"⠋", "⠙", "⠸"}

// TUIOpts holds dependencies and display config for the TUI (agent, session, model name, paths).
type TUIOpts struct {
	App          *agentapp.AgentApp
	Session      *agentapp.SessionContext
	ModelName    string
	Workspace    string
	SessionsDir  string
	Approval     agent.ApprovalHandler
	GlamourStyle string // "dark" or "light", detected once before the program starts
	RunStatus    agentapp.RunStatus
}

// agentDoneMsg is sent when the agent run finishes (in a tea.Cmd).
type agentDoneMsg struct {
	Result agentapp.RunResult
	Err    error
}

// streamDeltaMsg is sent when a content delta arrives from the streaming LLM.
type streamDeltaMsg struct {
	Delta string
}

// llmStartMsg is sent when a single LLM response starts inside an agent run.
type llmStartMsg struct{}

// llmEndMsg is sent when a single LLM response finishes inside an agent run.
type llmEndMsg struct {
	Content          string
	PromptTokens     int
	CompletionTokens int
}

type runStatusMsg struct {
	Status agentapp.RunStatus
}

// toolStartMsg is sent when the agent begins executing a tool call.
type toolStartMsg struct {
	CallID string
	Name   string
	Args   string
}

// toolEndMsg is sent when a tool call completes (success or error result).
type toolEndMsg struct {
	CallID   string
	Name     string
	Duration time.Duration
	IsError  bool
}

// toolDeniedMsg is sent when a tool call is blocked before execution.
type toolDeniedMsg struct {
	CallID string
	Name   string
	Reason string
}

// activeTool is one tool call in flight. Held as a slice keyed by call id
// rather than a single set of arguments: once several calls overlap, a result
// has to find its own arguments, and arrival order stops identifying them.
// Slice, not map, so the live view does not reorder between frames.
type activeTool struct {
	CallID string
	Name   string
	Args   string
}

// carouselTickMsg is sent by tea.Tick to advance the assistant "..." carousel.
type carouselTickMsg struct{}

// assistantRenderedMsg is sent when the off-loop markdown rendering of a completed
// assistant reply is finished. Rendering runs in a goroutine Cmd to avoid blocking
// the Bubble Tea event loop.
type assistantRenderedMsg struct {
	line           string
	continueStream bool
}

// Model is the root Bubble Tea model in transcript mode.
// Chat history lives in the terminal scrollback; this model only manages the
// bottom live strip: streaming preview, input, slash panels, approval, and footer.
type Model struct {
	opts             TUIOpts
	inputBlock       InputBlock
	busy             bool
	err              string // last error to show in footer
	width            int
	height           int
	carouselDots     int          // 0, 1, 2 for ".", "..", "..."
	focusInput       bool         // true = input has focus (slash panels may override)
	streamingBuffer  string       // current LLM response text while streaming
	activeTools      []activeTool // tool calls in flight, shown in the live view
	streamChannel    chan tea.Msg // receives streamDeltaMsg, tool events, and agentDoneMsg
	slashMCP         *slashMCPState
	slashModel       *slashModelState
	slashSkills      *slashSkillsState
	slashSession     *slashSessionState
	slashDiff        *slashDiffState
	slashPopup       *slashPopupState
	activePanel      slashPanel // generic panel (new abstraction); use openPanel/closeActivePanel
	branch           string
	userEmail        string
	pendingApproval  *approvalRequestMsg
	approvalSelected int
	runStatus        agentapp.RunStatus
}

// streamSinkToChannel implements agent.StreamSink by sending streamDeltaMsg to a channel.
type streamSinkToChannel struct {
	channel chan tea.Msg
}

func (s *streamSinkToChannel) OnDelta(delta string) {
	s.channel <- streamDeltaMsg{Delta: delta}
}

// eventSinkToChannel returns an agent.EventSink that forwards tool events to the stream channel.
func eventSinkToChannel(channel chan tea.Msg) func(agent.Event) {
	return func(e agent.Event) {
		switch e.Kind {
		case agent.EventLLMStart:
			channel <- runStatusMsg{Status: agentapp.RunStatus{
				ContextTokens:    e.ContextTokens,
				ContextWindow:    e.ContextWindow,
				PromptTokens:     e.PromptTokens,
				CompletionTokens: e.CompletionTokens,
			}}
			channel <- llmStartMsg{}
		case agent.EventLLMEnd:
			channel <- llmEndMsg{Content: e.Content, PromptTokens: e.PromptTokens, CompletionTokens: e.CompletionTokens}
		case agent.EventToolStart:
			channel <- toolStartMsg{CallID: e.ToolCallID, Name: e.ToolName, Args: e.ToolArgs}
		case agent.EventToolEnd:
			channel <- toolEndMsg{CallID: e.ToolCallID, Name: e.ToolName, Duration: e.ToolDuration, IsError: strings.HasPrefix(e.ToolResult, "error:")}
		case agent.EventToolDenied:
			channel <- toolDeniedMsg{CallID: e.ToolCallID, Name: e.ToolName, Reason: e.DenyReason}
		}
	}
}

// NewModel builds a TUI model with input block and stored opts.
func NewModel(opts TUIOpts) *Model {
	var userEmail string
	if info, err := auth.Info(); err == nil {
		userEmail = info.Email
	}
	return &Model{
		opts:       opts,
		inputBlock: NewInputBlock(),
		width:      80,
		height:     24,
		focusInput: true,
		branch:     git.CurrentBranch(opts.Workspace),
		userEmail:  userEmail,
		runStatus:  opts.RunStatus,
	}
}

// Init runs when the program starts; focuses input and starts cursor blink.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.inputBlock.Focus())
}

// FocusInput returns true when the input has focus (used by tests).
func (m *Model) FocusInput() bool {
	return m.focusInput
}

// runAgentWithStream starts the agent in a goroutine and returns a Cmd that reads the first event.
func runAgentWithStream(opts TUIOpts, text string, channel chan tea.Msg) tea.Cmd {
	sink := &streamSinkToChannel{channel: channel}
	evSink := eventSinkToChannel(channel)
	go func() {
		result, err := opts.App.RunPrompt(context.Background(), opts.Session, text, sink, opts.Approval, evSink)
		channel <- agentDoneMsg{Result: result, Err: err}
		close(channel)
	}()
	return func() tea.Msg { return <-channel }
}

func nextStreamMsgCmd(channel chan tea.Msg) tea.Cmd {
	if channel == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-channel
		if !ok {
			return nil
		}
		return msg
	}
}

func renderAssistantMsgCmd(text string, width int, style string, continueStream bool) tea.Cmd {
	return func() tea.Msg {
		return assistantRenderedMsg{line: formatAssistantMsgForScrollback(text, width, style), continueStream: continueStream}
	}
}

// renderStreamingPreview returns the last maxStreamPreviewLines of the streaming buffer
// for display in the live Bubble Tea view. Lines are truncated to terminal width so that
// Bubble Tea's cursor-movement math stays correct when the view shrinks after streaming ends.
func (m *Model) renderStreamingPreview() string {
	lines := strings.Split(m.streamingBuffer, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > maxStreamPreviewLines {
		lines = lines[len(lines)-maxStreamPreviewLines:]
	}
	// Reserve space for the "• " / "  " prefix (2 runes) plus a small margin.
	maxW := m.width - 4
	if maxW < 10 {
		maxW = 10
	}
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		runes := []rune(line)
		if len(runes) > maxW {
			line = string(runes[:maxW-1]) + "…"
		}
		if i == 0 {
			b.WriteString(assistantGlyphStyle.Render("◆ ") + line)
		} else {
			b.WriteString("  " + line)
		}
	}
	return b.String()
}

func handleKeyMsg(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Approval prompt intercepts all keys when a tool call is waiting for approval.
	if m.pendingApproval != nil {
		switch msg.String() {
		case "left", "h":
			if m.approvalSelected > 0 {
				m.approvalSelected--
			}
		case "right", "l":
			if m.approvalSelected < len(approvalChoices)-1 {
				m.approvalSelected++
			}
		case "enter":
			m.answerApproval(approvalChoices[m.approvalSelected].decision)
		case "y", "Y":
			m.answerApproval(agent.ApprovalAllowOnce)
		case "a", "A":
			m.answerApproval(agent.ApprovalAllowSession)
		case "n", "N", "esc":
			m.answerApproval(agent.ApprovalDeny)
		}
		return m, nil
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	// Tab: no-op in transcript mode (no viewport to toggle focus).
	if msg.Code == tea.KeyTab {
		return m, nil
	}
	// Active slashPanel (new abstraction) gets first dibs on keys.
	if m.activePanel != nil && !m.busy {
		if handled, cmd := m.activePanel.HandleKey(m, msg); handled {
			return m, cmd
		}
	}
	if m.focusInput && m.slashPopup != nil && len(m.slashPopup.matches) > 0 {
		switch msg.Code {
		case tea.KeyUp:
			if m.slashPopup.selected > 0 {
				m.slashPopup.selected--
			} else {
				m.slashPopup.selected = len(m.slashPopup.matches) - 1
			}
			return m, nil
		case tea.KeyDown:
			m.slashPopup.selected = (m.slashPopup.selected + 1) % len(m.slashPopup.matches)
			return m, nil
		}
	}
	switch msg.Code {
	case tea.KeyEscape:
		if m.busy {
			return m, nil
		}
		if m.slashPopup != nil {
			m.slashPopup = nil
			return m, nil
		}
		m.inputBlock.Reset()
		m.inputBlock.SyncHeight()
		return m, nil
	case tea.KeyEnter:
		if m.busy {
			return m, nil
		}
		if m.slashPopup != nil && len(m.slashPopup.matches) > 0 {
			cmd := m.slashPopup.matches[m.slashPopup.selected]
			m.slashPopup = nil
			m.inputBlock.Reset()
			m.inputBlock.SyncHeight()
			return dispatchSlashCommand(m, cmd)
		}
		text := strings.TrimSpace(m.inputBlock.Value())
		if text == "" {
			return m, nil
		}
		if strings.HasPrefix(text, "/") {
			m.inputBlock.Reset()
			m.inputBlock.SyncHeight()
			m.slashPopup = nil
			parts := strings.Fields(text)
			if len(parts) == 0 {
				return m, nil
			}
			return dispatchSlashCommand(m, parts[0], parts[1:]...)
		}
		m.inputBlock.Reset()
		m.inputBlock.SyncHeight()
		m.busy = true
		m.err = ""
		m.carouselDots = 0
		m.streamingBuffer = ""
		m.runStatus.PromptTokens = 0
		m.runStatus.CompletionTokens = 0
		channel := make(chan tea.Msg)
		m.streamChannel = channel
		// tea.Println returns a Cmd in Bubble Tea v2; use Sequence so the user message
		// appears in scrollback before the agent starts reading from the channel.
		userLine := formatUserMsgForScrollback(text)
		return m, tea.Sequence(
			tea.Println(userLine+"\n"),
			tea.Batch(
				tea.Tick(time.Duration(carouselTick)*time.Millisecond, func(t time.Time) tea.Msg { return carouselTickMsg{} }),
				runAgentWithStream(m.opts, text, channel),
			),
		)
	}
	// When the session panel is focused, route printable characters and backspace
	// to the search filter instead of the input block.
	cmd := m.inputBlock.Update(msg)
	m.inputBlock.SyncHeight()
	m.syncSlashPopupFromInput()
	return m, cmd
}

func handleWindowSize(m *Model, msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	inputW := m.width - 4
	if inputW < 8 {
		inputW = 8
	}
	m.inputBlock.SetWidth(inputW)
	m.inputBlock.SyncHeight()
	m.syncSlashPopupFromInput()
	return m, nil
}

func handleAgentDone(m *Model, msg agentDoneMsg) (tea.Model, tea.Cmd) {
	text := m.streamingBuffer
	width := m.width
	glamourStyle := m.opts.GlamourStyle
	m.busy = false
	m.carouselDots = 0
	m.streamingBuffer = ""
	m.activeTools = nil
	m.streamChannel = nil
	if msg.Err != nil {
		m.err = msg.Err.Error()
		slog.Error("agent failed", "err", msg.Err)
		return m, nil
	}
	m.runStatus = agentapp.RunStatus{
		ContextTokens:         msg.Result.ContextTokens,
		ContextWindow:         msg.Result.ContextWindow,
		PromptTokens:          msg.Result.PromptTokens,
		CompletionTokens:      msg.Result.CompletionTokens,
		TotalPromptTokens:     msg.Result.TotalPromptTokens,
		TotalCompletionTokens: msg.Result.TotalCompletionTokens,
	}
	if strings.TrimSpace(text) == "" {
		return m, nil
	}
	// Normally each LLM response is rendered on llmEndMsg. This fallback keeps a
	// successful run from dropping text if the stream closes without an LLM end event.
	return m, renderAssistantMsgCmd(text, width, glamourStyle, false)
}

func handleCarouselTick(m *Model, msg carouselTickMsg) (tea.Model, tea.Cmd) {
	if m.busy {
		m.carouselDots = (m.carouselDots + 1) % 3
		return m, tea.Tick(time.Duration(carouselTick)*time.Millisecond, func(t time.Time) tea.Msg { return carouselTickMsg{} })
	}
	return m, nil
}

func handleStreamDelta(m *Model, msg streamDeltaMsg) (tea.Model, tea.Cmd) {
	m.streamingBuffer += msg.Delta
	return m, nextStreamMsgCmd(m.streamChannel)
}

func handleLLMStart(m *Model, msg llmStartMsg) (tea.Model, tea.Cmd) {
	m.streamingBuffer = ""
	return m, nextStreamMsgCmd(m.streamChannel)
}

func handleLLMEnd(m *Model, msg llmEndMsg) (tea.Model, tea.Cmd) {
	m.runStatus = mergeRunStatus(m.runStatus, agentapp.RunStatus{
		PromptTokens:     msg.PromptTokens,
		CompletionTokens: msg.CompletionTokens,
	})
	text := m.streamingBuffer
	if strings.TrimSpace(text) == "" {
		text = msg.Content
	}
	m.streamingBuffer = ""
	if strings.TrimSpace(text) == "" {
		return m, nextStreamMsgCmd(m.streamChannel)
	}
	return m, renderAssistantMsgCmd(text, m.width, m.opts.GlamourStyle, true)
}

func mergeRunStatus(prev, next agentapp.RunStatus) agentapp.RunStatus {
	if next.ContextTokens == 0 {
		next.ContextTokens = prev.ContextTokens
	}
	if next.ContextWindow == 0 {
		next.ContextWindow = prev.ContextWindow
	}
	if next.TotalPromptTokens == 0 {
		next.TotalPromptTokens = prev.TotalPromptTokens
		if next.PromptTokens > prev.PromptTokens {
			next.TotalPromptTokens += next.PromptTokens - prev.PromptTokens
		}
	}
	if next.TotalCompletionTokens == 0 {
		next.TotalCompletionTokens = prev.TotalCompletionTokens
		if next.CompletionTokens > prev.CompletionTokens {
			next.TotalCompletionTokens += next.CompletionTokens - prev.CompletionTokens
		}
	}
	return next
}

func handleRunStatus(m *Model, msg runStatusMsg) (tea.Model, tea.Cmd) {
	m.runStatus = mergeRunStatus(m.runStatus, msg.Status)
	return m, nextStreamMsgCmd(m.streamChannel)
}

func updateRunStatusContext(prev, next agentapp.RunStatus) agentapp.RunStatus {
	next.PromptTokens = prev.PromptTokens
	next.CompletionTokens = prev.CompletionTokens
	next.TotalPromptTokens = prev.TotalPromptTokens
	next.TotalCompletionTokens = prev.TotalCompletionTokens
	return next
}

// finishTool removes a completed call from the live view and renders its
// transcript line. It matches on call id and falls back to the oldest call of
// the same name, so a surface that ever emits an event without an id degrades
// to today's behaviour instead of leaking a spinner.
func (m *Model) finishTool(callID, name, glyph, suffix string) string {
	idx := -1
	for i, t := range m.activeTools {
		if callID != "" && t.CallID == callID {
			idx = i
			break
		}
		if callID == "" && t.Name == name && idx < 0 {
			idx = i
		}
	}
	var args string
	if idx >= 0 {
		args = shortArgs(m.activeTools[idx].Args)
		m.activeTools = append(m.activeTools[:idx], m.activeTools[idx+1:]...)
	}
	line := "  " + glyph + " " + toolDisplayName(name)
	if args != "" {
		line += " (" + args + ")"
	}
	return line + suffix
}

// label renders one in-flight call. The spinner glyph is added in View().
func (t activeTool) label() string {
	if args := shortArgs(t.Args); args != "" {
		return toolDisplayName(t.Name) + " (" + args + ")"
	}
	return toolDisplayName(t.Name)
}

func handleToolStart(m *Model, msg toolStartMsg) (tea.Model, tea.Cmd) {
	m.activeTools = append(m.activeTools, activeTool(msg))
	return m, nextStreamMsgCmd(m.streamChannel)
}

func handleToolEnd(m *Model, msg toolEndMsg) (tea.Model, tea.Cmd) {
	glyph := toolGlyphSuccessStyle.Render("•")
	if msg.IsError {
		glyph = toolGlyphFailStyle.Render("•")
	}
	line := m.finishTool(msg.CallID, msg.Name, glyph, "")
	return m, tea.Sequence(tea.Println(line+"\n"), nextStreamMsgCmd(m.streamChannel))
}

func handleToolDenied(m *Model, msg toolDeniedMsg) (tea.Model, tea.Cmd) {
	line := m.finishTool(msg.CallID, msg.Name, toolGlyphFailStyle.Render("•"), " [denied]")
	return m, tea.Sequence(tea.Println(line+"\n"), nextStreamMsgCmd(m.streamChannel))
}

// approvalChoices is the prompt's outcome set, left to right. Session grants
// are what keep a per-write prompt from becoming something users turn off.
var approvalChoices = []struct {
	label    string
	decision agent.ApprovalDecision
}{
	{"Allow once(y)", agent.ApprovalAllowOnce},
	{"Allow session(a)", agent.ApprovalAllowSession},
	{"Deny(n)", agent.ApprovalDeny},
}

// answerApproval resolves the waiting tool call and clears the prompt.
func (m *Model) answerApproval(d agent.ApprovalDecision) {
	if m.pendingApproval == nil {
		return
	}
	m.pendingApproval.response <- d
	m.pendingApproval = nil
	m.approvalSelected = 0
}

// renderApprovalPanel renders the tool-approval prompt when a tool call is waiting.
func (m *Model) renderApprovalPanel() string {
	if m.pendingApproval == nil {
		return ""
	}
	keys := make([]string, 0, len(m.pendingApproval.Args))
	for k := range m.pendingApproval.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var argLines []string
	for _, k := range keys {
		line := fmt.Sprintf("  %s: %v", k, m.pendingApproval.Args[k])
		if len(line) > m.width-6 {
			line = line[:m.width-9] + "..."
		}
		argLines = append(argLines, line)
	}

	buttons := make([]string, len(approvalChoices))
	for i, c := range approvalChoices {
		if i == m.approvalSelected {
			buttons[i] = approvalSelectedStyle.Render(c.label)
			continue
		}
		buttons[i] = approvalUnselectedStyle.Render(c.label)
	}

	body := fmt.Sprintf("Tool: %s\n%s\n\n%s    ←→ select  enter: confirm",
		m.pendingApproval.ToolName,
		strings.Join(argLines, "\n"),
		strings.Join(buttons, "  "),
	)
	return approvalPanelStyle.Width(m.width - 4).Render(body)
}

func (m *Model) renderInputView() string {
	boxWidth := m.width - 2
	if boxWidth < 10 {
		boxWidth = 10
	}
	if m.busy {
		dots := []string{".", "..", "..."}
		return inputBoxStyle.Width(boxWidth).Render("Generating" + dots[m.carouselDots%3])
	}
	return inputBoxStyle.Width(boxWidth).Render(m.inputBlock.View())
}

func (m *Model) renderFooterView() string {
	workspacePart := "@" + m.opts.Workspace
	if m.branch != "" {
		workspacePart += " (|-" + m.branch + ")"
	}
	currentModel := m.opts.ModelName
	if m.opts.Session != nil {
		currentModel = m.opts.Session.ModelName(currentModel)
	}
	line1 := footerModelStyle.Render("model: "+currentModel) + " | " +
		footerBranchStyle.Render(workspacePart)
	if tag := sandboxFooterTag(m.opts.App); tag != "" {
		line1 += " | " + tag
	}
	if m.userEmail != "" {
		line1 += " | " + m.userEmail
	}

	line2 := formatRunStatus(m.runStatus) + " | ctrl+c: quit | esc: clear/dismiss | /… + ↑↓: commands"
	if m.activePanel != nil {
		if hint := m.activePanel.FooterHint(); hint != "" {
			line2 += " | " + hint
		}
	}
	if m.err != "" {
		line2 += " | error: " + m.err
	}
	return line1 + "\n" + line2
}

func formatRunStatus(st agentapp.RunStatus) string {
	ctx := "ctx: unknown"
	if st.ContextWindow > 0 {
		percent := 0
		if st.ContextTokens > 0 {
			percent = int(float64(st.ContextTokens)/float64(st.ContextWindow)*100 + 0.5)
		}
		ctx = fmt.Sprintf("ctx: %d%% (%s/%s)", percent, formatTokenCount(st.ContextTokens), formatTokenCount(st.ContextWindow))
	}
	return ctx + " | " + formatTokenUsage(st.PromptTokens, st.CompletionTokens, st.TotalPromptTokens, st.TotalCompletionTokens)
}

func formatTokenUsage(promptTokens, completionTokens, totalPromptTokens, totalCompletionTokens int) string {
	return "tokens(in/out): " + formatTokenUsageValue(promptTokens, completionTokens, totalPromptTokens, totalCompletionTokens)
}

func formatTokenUsageValue(promptTokens, completionTokens, totalPromptTokens, totalCompletionTokens int) string {
	usage := fmt.Sprintf("%s/%s", formatTokenCount(promptTokens), formatTokenCount(completionTokens))
	if totalPromptTokens > 0 || totalCompletionTokens > 0 {
		usage += fmt.Sprintf(" (%s/%s)", formatTokenCount(totalPromptTokens), formatTokenCount(totalCompletionTokens))
	}
	return usage
}

func formatTokenCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n%1000 == 0 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

// Update handles messages: keys (quit, submit, focus toggle), resize, agent events.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return handleKeyMsg(m, msg)
	case tea.WindowSizeMsg:
		return handleWindowSize(m, msg)
	case agentDoneMsg:
		return handleAgentDone(m, msg)
	case llmStartMsg:
		return handleLLMStart(m, msg)
	case llmEndMsg:
		return handleLLMEnd(m, msg)
	case runStatusMsg:
		return handleRunStatus(m, msg)
	case streamDeltaMsg:
		return handleStreamDelta(m, msg)
	case toolStartMsg:
		return handleToolStart(m, msg)
	case toolEndMsg:
		return handleToolEnd(m, msg)
	case toolDeniedMsg:
		return handleToolDenied(m, msg)
	case carouselTickMsg:
		return handleCarouselTick(m, msg)
	case approvalRequestMsg:
		m.pendingApproval = &msg
		m.approvalSelected = 0
		return m, nil
	case assistantRenderedMsg:
		if msg.line == "" {
			if msg.continueStream {
				return m, nextStreamMsgCmd(m.streamChannel)
			}
			return m, nil
		}
		if msg.continueStream {
			return m, tea.Sequence(tea.Println(msg.line+"\n"), nextStreamMsgCmd(m.streamChannel))
		}
		return m, tea.Println(msg.line + "\n")
	default:
		// tea.Println returns a Cmd whose result is a tea-internal printLineMessage.
		// That message is handled by the renderer (insertAbove) but also reaches
		// model.Update. Forwarding it to the textarea causes the v2 bubbles viewport
		// to insert parts of the ANSI body as text via its default key handler.
		// Filter it out by type name before passing anything to the textarea.
		if fmt.Sprintf("%T", msg) == "tea.printLineMessage" {
			return m, nil
		}
		cmd := m.inputBlock.Update(msg)
		m.inputBlock.SyncHeight()
		m.syncSlashPopupFromInput()
		return m, cmd
	}
}

// View renders only the bottom live strip: streaming preview, slash panels, approval, input, footer.
// Chat history lives in the terminal scrollback and is not rendered here.
func (m *Model) View() tea.View {
	var parts []string

	// Streaming preview: show tail of buffer above the input while busy.
	if m.busy {
		if m.streamingBuffer != "" {
			parts = append(parts, m.renderStreamingPreview())
		}
		if len(m.activeTools) > 0 {
			spinner := toolGlyphPendingStyle.Render(toolSpinnerFrames[m.carouselDots])
			for _, t := range m.activeTools {
				parts = append(parts, "  "+spinner+" "+t.label())
			}
		}
	}

	// Slash panels (only shown when not busy).
	if !m.busy {
		if s := m.renderActivePanel(); s != "" {
			parts = append(parts, s)
		}
		if s := m.renderSlashPopupPanel(); s != "" {
			parts = append(parts, s)
		}
	}

	// Approval panel is shown regardless of busy state.
	if s := m.renderApprovalPanel(); s != "" {
		parts = append(parts, s)
	}

	parts = append(parts, m.renderInputView())
	parts = append(parts, m.renderFooterView())
	return tea.NewView(strings.Join(parts, "\n"))
}
