package cli

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// approvalRequestMsg is sent by TUIApprovalHandler to the Tea program when a tool call
// needs interactive approval. The agent goroutine blocks on response until the user responds.
type approvalRequestMsg struct {
	ToolName string
	Args     map[string]any
	response chan agent.ApprovalDecision
}

// TUIApprovalHandler implements agent.ApprovalHandler for the Bubble Tea TUI.
// Create it before the program, wire the program in after tea.NewProgram.
type TUIApprovalHandler struct {
	program *tea.Program
}

func NewTUIApprovalHandler() *TUIApprovalHandler {
	return &TUIApprovalHandler{}
}

func (h *TUIApprovalHandler) SetProgram(p *tea.Program) {
	h.program = p
}

// RequestApproval sends an approval request to the TUI and blocks until the user
// answers: y (once), a (session), or n/Esc (deny).
func (h *TUIApprovalHandler) RequestApproval(name string, args map[string]any) agent.ApprovalDecision {
	if h.program == nil {
		return agent.ApprovalDeny
	}
	respCh := make(chan agent.ApprovalDecision, 1)
	h.program.Send(approvalRequestMsg{
		ToolName: name,
		Args:     args,
		response: respCh,
	})
	return <-respCh
}
