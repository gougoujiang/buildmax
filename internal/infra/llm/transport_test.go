package llm

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (f httpDoerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBuildMaxUserAgentPreservesGatewayCallOrigin(t *testing.T) {
	var got string
	client := withBuildMaxUserAgent(&http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		got = req.Header.Get("User-Agent")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}, "server")
	req := httptestNewRequest(t, cllm.WithCallOrigin(context.Background(), cllm.CallOrigin{Surface: "cli", ViaGateway: true}))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if want := config.UserAgent("cli", true); got != want {
		t.Errorf("User-Agent = %q, want %q", got, want)
	}
}

func TestUsageCaptureHTTPClientCapturesSSEUsage(t *testing.T) {
	usage := &cllm.Usage{}
	req := httptestNewRequest(t, context.WithValue(context.Background(), streamUsageKey, usage))
	client := &usageCaptureHTTPClient{base: httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			Header: http.Header{"Content-Type": {"text/event-stream"}},
			Body:   io.NopCloser(strings.NewReader("data: {\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":5,\"total_tokens\":8}}\n\n")),
		}, nil
	})}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if *usage != (cllm.Usage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8}) {
		t.Errorf("usage = %+v, want prompt=3 completion=5 total=8", *usage)
	}
}

func httptestNewRequest(t *testing.T, ctx context.Context) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	return req
}
