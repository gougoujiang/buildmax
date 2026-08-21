package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// HTTPBlockingStatus is the HTTP status code an endpoint can use to deny an
// action without producing a structured JSON response. Picked to match
// "Unprocessable Entity" semantics; mirrors the spirit of CommandDriver's
// exit-2 contract.
const HTTPBlockingStatus = 422

// HTTPDriver posts the HookInput JSON to entry.URL and parses the response.
//
//   - 2xx with no body / non-JSON body: allow.
//   - 2xx with JSON {"decision":"block","reason":"..."}: block.
//   - HTTPBlockingStatus (422): block with the response body as reason.
//   - Any other non-2xx, network error, or timeout: fail open with a warn.
//
// Header values may contain "$VAR" / "${VAR}" placeholders. Only env vars
// listed in entry.AllowedEnv are substituted — this prevents accidental
// leakage of arbitrary process env into outbound requests.
type HTTPDriver struct {
	// Client is the http.Client used for outbound requests. Tests inject
	// their own; production constructs one with the per-entry timeout.
	Client *http.Client
}

// NewHTTPDriver returns a driver with a default client. Per-entry timeout is
// applied via context, so the client itself does not need a global timeout.
func NewHTTPDriver() *HTTPDriver {
	return &HTTPDriver{Client: &http.Client{}}
}

// Type satisfies Driver.
func (HTTPDriver) Type() string { return config.HookTypeHTTP }

// Run executes one HTTP hook entry.
func (d *HTTPDriver) Run(ctx context.Context, entry config.HookEntry, in agent.HookInput) agent.HookOutput {
	if entry.URL == "" {
		componentLog().Warn("http entry missing url", "event", in.Event)
		return agent.HookOutput{}
	}
	body, err := json.Marshal(in)
	if err != nil {
		componentLog().Warn("marshal http body failed; failing open", "event", in.Event, "err", err)
		return agent.HookOutput{}
	}

	reqCtx, cancel := context.WithTimeout(ctx, resolveTimeout(entry.Timeout))
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, entry.URL, bytes.NewReader(body))
	if err != nil {
		componentLog().Warn("build http request failed; failing open", "event", in.Event, "url", entry.URL, "err", err)
		return agent.HookOutput{}
	}
	req.Header.Set("Content-Type", "application/json")
	env := allowedEnvLookup(entry.AllowedEnv)
	for k, v := range entry.Headers {
		req.Header.Set(k, expandEnv(v, env))
	}

	start := time.Now()
	resp, err := d.Client.Do(req)
	dur := time.Since(start)
	if err != nil {
		componentLog().Warn("http request failed; failing open",
			"event", in.Event,
			"url", entry.URL,
			"dur", dur,
			"err", err,
		)
		return agent.HookOutput{}
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		componentLog().Warn("read http response failed; failing open", "event", in.Event, "url", entry.URL, "err", readErr)
		return agent.HookOutput{}
	}

	switch {
	case resp.StatusCode == HTTPBlockingStatus:
		reason := strings.TrimSpace(string(respBody))
		if reason == "" {
			reason = fmt.Sprintf("hook %q returned HTTP %d", entry.URL, resp.StatusCode)
		}
		// A 422 body may still be JSON with a reason; prefer that when present.
		if parsed, ok := parseHookOutput(respBody); ok {
			if parsed.Reason != "" {
				reason = parsed.Reason
			}
		}
		componentLog().Info("http blocked via 422", "event", in.Event, "tool", in.ToolName, "url", entry.URL, "reason", reason, "dur", dur)
		return agent.HookOutput{Decision: agent.HookDecisionBlock, Reason: reason}
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		out, ok := parseHookOutput(respBody)
		if !ok {
			componentLog().Debug("http ok", "event", in.Event, "url", entry.URL, "dur", dur)
			return agent.HookOutput{}
		}
		if out.Blocked() {
			componentLog().Info("http blocked via json", "event", in.Event, "tool", in.ToolName, "url", entry.URL, "reason", out.Reason)
		}
		return out
	default:
		componentLog().Warn("http non-2xx; failing open",
			"event", in.Event,
			"url", entry.URL,
			"status", resp.StatusCode,
			"body", truncate(string(respBody), 500),
			"dur", dur,
		)
		return agent.HookOutput{}
	}
}

// allowedEnvLookup returns a function that resolves only the env vars
// whitelisted in the entry. Anything outside the whitelist returns "".
func allowedEnvLookup(allowed []string) func(string) string {
	if len(allowed) == 0 {
		return func(string) string { return "" }
	}
	set := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		set[name] = struct{}{}
	}
	return func(name string) string {
		if _, ok := set[name]; !ok {
			return ""
		}
		return os.Getenv(name)
	}
}

// expandEnv replaces "$VAR" and "${VAR}" with the lookup result. Unknown or
// disallowed names expand to the empty string (matching os.Expand).
func expandEnv(s string, lookup func(string) string) string {
	return os.Expand(s, lookup)
}
