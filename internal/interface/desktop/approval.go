package desktop

import (
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

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
func (h *DesktopApprovalHandler) RequestApproval(name string, args map[string]any) agent.ApprovalDecision {
	h.app.mu.Lock()
	ctx := h.app.ctx
	h.app.mu.Unlock()
	if ctx == nil {
		return agent.ApprovalDeny
	}

	respCh := make(chan agent.ApprovalDecision, 1)
	h.mu.Lock()
	h.pending = respCh
	h.mu.Unlock()

	runtime.EventsEmit(ctx, eventApprovalRequest, &ApprovalRequestPayload{
		ProjectID: h.projectID,
		ToolName:  name,
		Args:      args,
	})

	return <-respCh
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
