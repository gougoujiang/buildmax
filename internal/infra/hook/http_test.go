package hook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
)

// TestHTTPDriver_2xxNoBodyAllows asserts that a 200 OK with no body is
// treated as allow.
func TestHTTPDriver_2xxNoBodyAllows(t *testing.T) {
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

// TestHTTPDriver_JSONBlocks asserts that a 200 with a JSON body
// {"decision":"block","reason":"..."} is honored.
func TestHTTPDriver_JSONBlocks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"decision": "block", "reason": "service says no"})
	}))
	t.Cleanup(srv.Close)

	d := NewHTTPDriver(nil)
	out := d.Run(context.Background(),
		corehook.Entry{URL: srv.URL},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"},
	)
	if !out.Blocked() {
		t.Fatalf("expected block, got %+v", out)
	}
	if out.Reason != "service says no" {
		t.Errorf("reason = %q, want %q", out.Reason, "service says no")
	}
}

// TestHTTPDriver_422Blocks asserts that an HTTPBlockingStatus response with a
// plain-text body surfaces the body as the reason.
func TestHTTPDriver_422Blocks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(HTTPBlockingStatus)
		_, _ = w.Write([]byte("policy violation"))
	}))
	t.Cleanup(srv.Close)

	d := NewHTTPDriver(nil)
	out := d.Run(context.Background(),
		corehook.Entry{URL: srv.URL},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"},
	)
	if !out.Blocked() {
		t.Fatalf("expected block, got %+v", out)
	}
	if out.Reason != "policy violation" {
		t.Errorf("reason = %q, want %q", out.Reason, "policy violation")
	}
}

// TestHTTPDriver_5xxFailsOpen asserts that a 500 from the server fails open.
func TestHTTPDriver_5xxFailsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	d := NewHTTPDriver(nil)
	out := d.Run(context.Background(),
		corehook.Entry{URL: srv.URL},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"},
	)
	if out.Blocked() {
		t.Errorf("expected allow on 5xx, got %+v", out)
	}
}

// TestHTTPDriver_HeaderInterpolationWhitelist asserts that $VAR placeholders
// in headers are only substituted for env vars listed in allowed_env.
func TestHTTPDriver_HeaderInterpolationWhitelist(t *testing.T) {
	t.Setenv("HOOK_ALLOWED", "secret-1")
	t.Setenv("HOOK_FORBIDDEN", "secret-2")

	got := make(chan http.Header, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	d := NewHTTPDriver(nil)
	d.Run(context.Background(),
		corehook.Entry{
			URL:        srv.URL,
			Headers:    map[string]string{"X-Allowed": "$HOOK_ALLOWED", "X-Forbidden": "$HOOK_FORBIDDEN"},
			AllowedEnv: []string{"HOOK_ALLOWED"},
		},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"},
	)
	hdr := <-got
	if hdr.Get("X-Allowed") != "secret-1" {
		t.Errorf("X-Allowed = %q, want secret-1", hdr.Get("X-Allowed"))
	}
	if hdr.Get("X-Forbidden") != "" {
		t.Errorf("X-Forbidden = %q, want empty (not whitelisted)", hdr.Get("X-Forbidden"))
	}
}

// TestHTTPDriver_PostsHookInputAsBody asserts the payload arrives intact.
func TestHTTPDriver_PostsHookInputAsBody(t *testing.T) {
	got := make(chan agent.HookInput, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var hi agent.HookInput
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &hi)
		got <- hi
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	d := NewHTTPDriver(nil)
	d.Run(context.Background(),
		corehook.Entry{URL: srv.URL},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "writefile", SessionID: "s1"},
	)
	hi := <-got
	if hi.Event != agent.HookPreToolUse || hi.ToolName != "writefile" || hi.SessionID != "s1" {
		t.Errorf("posted body = %+v", hi)
	}
}

func TestHTTPDriver_Type(t *testing.T) {
	if NewHTTPDriver(nil).Type() != corehook.TypeHTTP {
		t.Errorf("Type() = %q", NewHTTPDriver(nil).Type())
	}
}
