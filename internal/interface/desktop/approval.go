package desktop

import (
	"context"
	"sync"

	"github.com/gougoujiang/buildmax/internal/core/agent"
)

const eventApprovalRequest = "desktop/approval-request"

// ApprovalRequestPayload is emitted to the frontend when a tool call needs approval.
type ApprovalRequestPayload struct {
	ProjectID string         `json:"project_id"`
	ToolName  string         `json:"tool_name"`
	Args      map[string]any `json:"args"`
}

// DesktopApprovalHandler implements agent.ApprovalHandler for the Wails desktop app.
// It emits a Wails event to the frontend and blocks until RespondApproval is called.
type DesktopApprovalHandler struct {
	app       *App
	projectID string
	mu        sync.Mutex
	pending   chan agent.ApprovalDecision // non-nil while waiting for frontend response
}

func newDesktopApprovalHandler(app *App, projectID string) *DesktopApprovalHandler {
	return &DesktopApprovalHandler{app: app, projectID: projectID}
}

// RequestApproval emits an approval-request event to the frontend and blocks until
// the user responds via RespondApproval. Denies if the app context is not ready.
func (h *DesktopApprovalHandler) RequestApproval(ctx context.Context, name string, args map[string]any) agent.ApprovalDecision {
	h.app.mu.Lock()
	uiCtx := h.app.ctx // Wails context for emitting, distinct from the run's ctx
	h.app.mu.Unlock()
	if uiCtx == nil {
		return agent.ApprovalDeny
	}

	respCh := make(chan agent.ApprovalDecision, 1)
	h.mu.Lock()
	h.pending = respCh
	h.mu.Unlock()

	h.app.emit(uiCtx, eventApprovalRequest, &ApprovalRequestPayload{
		ProjectID: h.projectID,
		ToolName:  name,
		Args:      args,
	})

	select {
	case d := <-respCh:
		return d
	case <-ctx.Done():
		// Cancelled with the prompt still up. Without this the run goroutine
		// waits forever on an answer nobody will give, its deferred cleanup
		// never runs, and the project stays permanently "already in progress".
		h.mu.Lock()
		if h.pending == respCh {
			h.pending = nil
		}
		h.mu.Unlock()
		return agent.ApprovalDeny
	}
}

// respond resolves a pending approval request. Called by App.RespondApproval.
func (h *DesktopApprovalHandler) respond(decision agent.ApprovalDecision) {
	h.mu.Lock()
	pending := h.pending
	h.pending = nil
	h.mu.Unlock()
	if pending != nil {
		pending <- decision
	}
}
