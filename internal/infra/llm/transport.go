package llm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// contextKey is used as the key type for stream usage values stored in context.
type contextKey struct{}

var streamUsageKey = &contextKey{}

// usageCaptureTransport wraps an HTTP RoundTripper to intercept SSE streams and
// extract token usage from raw "data:" chunks (providers that embed usage in SSE).
type usageCaptureTransport struct {
	base http.RoundTripper
}

func (t *usageCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		return resp, nil
	}
	if v := req.Context().Value(streamUsageKey); v != nil {
		if usage, ok := v.(*cllm.Usage); ok && usage != nil {
			resp.Body = &usageCaptureReader{body: resp.Body, usage: usage}
		}
	}
	return resp, nil
}

// usageCaptureReader tees the SSE stream and parses "data:" lines for token usage.
type usageCaptureReader struct {
	body  io.ReadCloser
	usage *cllm.Usage
	buf   []byte
	mu    sync.Mutex
}

func (r *usageCaptureReader) Read(p []byte) (n int, err error) {
	n, err = r.body.Read(p)
	if n > 0 {
		r.mu.Lock()
		r.buf = append(r.buf, p[:n]...)
		r.parseUsage()
		r.mu.Unlock()
	}
	return n, err
}

func (r *usageCaptureReader) Close() error {
	return r.body.Close()
}

func (r *usageCaptureReader) parseUsage() {
	const dataPrefix = "data: "
	for {
		idx := bytes.Index(r.buf, []byte("\n\n"))
		if idx < 0 {
			break
		}
		event := r.buf[:idx]
		r.buf = r.buf[idx+2:]
		lines := bytes.Split(event, []byte("\n"))
		for _, line := range lines {
			line = bytes.TrimSpace(line)
			if !bytes.HasPrefix(line, []byte(dataPrefix)) {
				continue
			}
			jsonBytes := line[len(dataPrefix):]
			if string(jsonBytes) == "[DONE]" {
				continue
			}
			var obj struct {
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage,omitempty"`
			}
			if json.Unmarshal(jsonBytes, &obj) != nil || obj.Usage == nil {
				continue
			}
			r.usage.PromptTokens = obj.Usage.PromptTokens
			r.usage.CompletionTokens = obj.Usage.CompletionTokens
			r.usage.TotalTokens = obj.Usage.TotalTokens
		}
	}
}
