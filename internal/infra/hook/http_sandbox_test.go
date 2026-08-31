package hook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
)

// TestHTTPDriver_HostDeniedBlocksWithoutRequest asserts a sandbox denial
// blocks the hook and never reaches the network — an http hook is a
// non-bash egress path exactly like WebFetch, so it honors the same
// allow-list rather than reaching the network unconstrained.
func TestHTTPDriver_HostDeniedBlocksWithoutRequest(t *testing.T) {
	requested := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requested = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	stub := &stubSandbox{enabled: true, hostAllowed: false, hostReason: "sandbox: host denied"}
	d := NewHTTPDriver(stub)
	out := d.Run(context.Background(),
		corehook.Entry{URL: srv.URL},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"},
	)
	if !out.Blocked() {
		t.Fatalf("expected block, got %+v", out)
	}
	if out.Reason != "sandbox: host denied" {
		t.Errorf("reason = %q, want the sandbox's own reason", out.Reason)
	}
	if requested {
		t.Error("the denied host was reached anyway")
	}
}

// TestHTTPDriver_HostAllowedProceeds asserts an allowed host is unaffected:
// the request still reaches the server and its own response drives the
// decision.
func TestHTTPDriver_HostAllowedProceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	stub := &stubSandbox{enabled: true, hostAllowed: true}
	d := NewHTTPDriver(stub)
	out := d.Run(context.Background(),
		corehook.Entry{URL: srv.URL},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"},
	)
	if out.Blocked() {
		t.Errorf("expected allow, got %+v", out)
	}
}

// TestHTTPDriver_DisabledSandboxSkipsCheck asserts a disabled sandbox (the
// NoopSandbox default from NewHTTPDriver(nil)) behaves exactly as before
// this change: no host check at all.
func TestHTTPDriver_DisabledSandboxSkipsCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	d := NewHTTPDriver(nil)
	out := d.Run(context.Background(),
		corehook.Entry{URL: srv.URL},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"},
	)
	if out.Blocked() {
		t.Errorf("expected allow, got %+v", out)
	}
}
