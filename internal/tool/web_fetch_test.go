package tool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// mockWebFetchLLM is a fake LLMClient for WebFetch tests.
type mockWebFetchLLM struct {
	mu        sync.Mutex
	reply     string
	callCount int
}

func (m *mockWebFetchLLM) ChatCompletionBlocking(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (string, []llm.ToolCall, llm.Usage, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	return m.reply, nil, llm.Usage{}, nil
}

func (m *mockWebFetchLLM) ChatCompletionStreaming(ctx context.Context, messages []llm.Message, tools []llm.ToolDef, onDelta func(string)) (string, []llm.ToolCall, llm.Usage, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	if onDelta != nil && m.reply != "" {
		onDelta(m.reply)
	}
	return m.reply, nil, llm.Usage{}, nil
}

func (m *mockWebFetchLLM) ContextWindow() int { return 0 }

func (m *mockWebFetchLLM) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func TestWebFetch_NilLLMClient_FetchWithoutPromptOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plain"))
	}))
	defer server.Close()

	w := NewWebFetch(nil, 15*time.Minute)
	if w == nil {
		t.Fatal("NewWebFetch(nil) returned nil")
	}
	ctx := context.Background()
	got, err := w.Execute(ctx, map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("Execute without prompt: %v", err)
	}
	if got != "plain" {
		t.Errorf("got %q, want plain", got)
	}
}

func TestWebFetch_NilLLMClient_PromptRequiresClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("body"))
	}))
	defer server.Close()

	w := NewWebFetch(nil, 15*time.Minute)
	ctx := context.Background()
	_, err := w.Execute(ctx, map[string]any{"url": server.URL, "prompt": "summarize"})
	if err == nil {
		t.Fatal("Execute with prompt but nil LLM should return error")
	}
	if !strings.Contains(err.Error(), "LLM client") {
		t.Errorf("error should mention LLM client: %v", err)
	}
}

func TestWebFetch_Execute_MissingURL(t *testing.T) {
	mock := &mockWebFetchLLM{reply: "ok"}
	w := NewWebFetch(mock, 15*time.Minute)
	ctx := context.Background()
	_, err := w.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute with no url should return error")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("error should mention url: %v", err)
	}
}

func TestWebFetch_Execute_InvalidURL(t *testing.T) {
	mock := &mockWebFetchLLM{reply: "ok"}
	w := NewWebFetch(mock, 15*time.Minute)
	ctx := context.Background()
	_, err := w.Execute(ctx, map[string]any{"url": "://no-host"})
	if err == nil {
		t.Fatal("Execute with invalid url should return error")
	}
}

func TestWebFetch_Execute_HTTPUpgradedToHTTPS(t *testing.T) {
	// normalizeURL is used before fetch; we test it upgrades http to https
	u, err := normalizeURL("http://example.com/path")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "https://") {
		t.Errorf("normalized URL should be https: %s", u)
	}
	if u != "https://example.com/path" {
		t.Errorf("normalized URL = %s, want https://example.com/path", u)
	}
}

func TestWebFetch_Execute_SameHostRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redir" {
			http.Redirect(w, r, "/content", http.StatusFound)
			return
		}
		if r.URL.Path == "/content" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("final content"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	mock := &mockWebFetchLLM{reply: "ok"}
	w := NewWebFetch(mock, 15*time.Minute)
	ctx := context.Background()
	url := server.URL + "/redir"
	result, err := w.Execute(ctx, map[string]any{"url": url})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "final content" {
		t.Errorf("result = %q, want %q", result, "final content")
	}
}

func TestWebFetch_Execute_CrossHostRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to a different host (example.com)
		http.Redirect(w, r, "https://example.com/other", http.StatusFound)
	}))
	defer server.Close()

	mock := &mockWebFetchLLM{reply: "ok"}
	w := NewWebFetch(mock, 15*time.Minute)
	ctx := context.Background()
	result, err := w.Execute(ctx, map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "Redirected to a different host") {
		t.Errorf("result should contain redirect message: %s", result)
	}
	if !strings.Contains(result, "https://example.com/other") {
		t.Errorf("result should contain redirect URL: %s", result)
	}
}

func TestWebFetch_Execute_CacheHit(t *testing.T) {
	reqCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("cached body"))
	}))
	defer server.Close()

	mock := &mockWebFetchLLM{reply: "ok"}
	w := NewWebFetch(mock, 15*time.Minute)
	ctx := context.Background()
	url := server.URL + "/cache-test"

	result1, err := w.Execute(ctx, map[string]any{"url": url})
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if result1 != "cached body" {
		t.Errorf("first result = %q", result1)
	}
	if reqCount != 1 {
		t.Errorf("after first request, reqCount = %d, want 1", reqCount)
	}

	result2, err := w.Execute(ctx, map[string]any{"url": url})
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if result2 != "cached body" {
		t.Errorf("second result = %q", result2)
	}
	if reqCount != 1 {
		t.Errorf("cache hit should not refetch: reqCount = %d, want 1", reqCount)
	}
}

func TestWebFetch_Execute_CacheExpiry(t *testing.T) {
	reqCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		_, _ = w.Write([]byte("body"))
	}))
	defer server.Close()

	mock := &mockWebFetchLLM{reply: "ok"}
	w := NewWebFetch(mock, 1*time.Millisecond)
	ctx := context.Background()
	url := server.URL

	_, err := w.Execute(ctx, map[string]any{"url": url})
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if reqCount != 1 {
		t.Errorf("reqCount = %d, want 1", reqCount)
	}

	time.Sleep(2 * time.Millisecond)
	_, err = w.Execute(ctx, map[string]any{"url": url})
	if err != nil {
		t.Fatalf("second Execute (after expiry): %v", err)
	}
	if reqCount != 2 {
		t.Errorf("after expiry should refetch: reqCount = %d, want 2", reqCount)
	}
}

func TestWebFetch_Execute_HTMLConverted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><h1>Hello</h1><p>World</p></body></html>"))
	}))
	defer server.Close()

	mock := &mockWebFetchLLM{reply: "ok"}
	w := NewWebFetch(mock, 15*time.Minute)
	ctx := context.Background()
	result, err := w.Execute(ctx, map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// k3a html2text strips tags; we expect "Hello" and "World" in output
	if !strings.Contains(result, "Hello") || !strings.Contains(result, "World") {
		t.Errorf("HTML should be converted to text containing Hello and World: %s", result)
	}
}

func TestWebFetch_Execute_EmptyPromptReturnsRawContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("raw content"))
	}))
	defer server.Close()

	mock := &mockWebFetchLLM{reply: "LLM reply"}
	w := NewWebFetch(mock, 15*time.Minute)
	ctx := context.Background()
	result, err := w.Execute(ctx, map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "raw content" {
		t.Errorf("result = %q, want raw content (no prompt => no LLM call)", result)
	}
	if mock.getCallCount() != 0 {
		t.Errorf("LLM should not be called when prompt is empty: callCount = %d", mock.getCallCount())
	}
}

func TestWebFetch_Execute_NonEmptyPromptCallsLLM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("page content"))
	}))
	defer server.Close()

	mock := &mockWebFetchLLM{reply: "summary from LLM"}
	w := NewWebFetch(mock, 15*time.Minute)
	ctx := context.Background()
	result, err := w.Execute(ctx, map[string]any{"url": server.URL, "prompt": "Summarize this"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "summary from LLM" {
		t.Errorf("result = %q, want summary from LLM", result)
	}
	if mock.getCallCount() != 1 {
		t.Errorf("LLM should be called once: callCount = %d", mock.getCallCount())
	}
}

func TestWebFetch_Execute_OversizedContentTruncated(t *testing.T) {
	// Create content larger than MaxContentRunes
	big := strings.Repeat("x", MaxContentRunes+1000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(big))
	}))
	defer server.Close()

	mock := &mockWebFetchLLM{reply: "ok"}
	w := NewWebFetch(mock, 15*time.Minute)
	ctx := context.Background()
	result, err := w.Execute(ctx, map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "content truncated") {
		t.Errorf("result should contain truncation note: len(result)=%d", len(result))
	}
	if !strings.Contains(result, "total") || !strings.Contains(result, "characters") {
		t.Errorf("result should mention total characters: %s", result)
	}
	runes := 0
	for range result {
		runes++
	}
	if runes > MaxContentRunes+200 {
		t.Errorf("truncated result should be ~MaxContentRunes + note length, got %d runes", runes)
	}
}

func TestWebFetch_NameAndParameters(t *testing.T) {
	mock := &mockWebFetchLLM{}
	w := NewWebFetch(mock, 15*time.Minute)
	if w.Name() != ToolNameWebFetch {
		t.Errorf("Name() = %q, want WebFetch", w.Name())
	}
	params := w.Parameters()
	m, ok := params.(map[string]any)
	if !ok {
		t.Fatalf("Parameters() should return map[string]any")
	}
	req, ok := m["required"].([]string)
	if !ok || len(req) == 0 {
		t.Errorf("required should include url")
	}
	// Ensure WebFetch implements llm.Tool
	var _ llm.Tool = (*WebFetch)(nil)
}
