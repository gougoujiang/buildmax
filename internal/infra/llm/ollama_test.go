package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// ollamaUpstream is a fake daemon that answers /api/chat, /api/tags, and
// /api/show, and remembers what each was asked.
type ollamaUpstream struct {
	server   *httptest.Server
	bodies   map[string][]string
	requests map[string]int
	show     string
	tags     string
	chat     string
	status   int
}

func newOllamaUpstream(t *testing.T) *ollamaUpstream {
	t.Helper()
	up := &ollamaUpstream{
		bodies:   map[string][]string{},
		requests: map[string]int{},
		chat:     ollamaBody(reply{text: "ok"}),
	}
	up.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body := make([]byte, req.ContentLength)
		if req.ContentLength > 0 {
			_, _ = req.Body.Read(body)
		}
		up.bodies[req.URL.Path] = append(up.bodies[req.URL.Path], string(body))
		up.requests[req.URL.Path]++

		w.Header().Set("Content-Type", "application/json")
		if up.status != 0 && req.URL.Path == "/api/chat" {
			w.WriteHeader(up.status)
			_, _ = w.Write([]byte(`{"error":"model \"m\" not found, try pulling it first"}`))
			return
		}
		switch req.URL.Path {
		case "/api/show":
			if up.show == "" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"model not found"}`))
				return
			}
			_, _ = w.Write([]byte(up.show))
		case "/api/tags":
			_, _ = w.Write([]byte(up.tags))
		default:
			_, _ = w.Write([]byte(up.chat))
		}
	}))
	t.Cleanup(up.server.Close)
	return up
}

func (u *ollamaUpstream) lastChatRequest(t *testing.T) map[string]any {
	t.Helper()
	bodies := u.bodies["/api/chat"]
	if len(bodies) == 0 {
		t.Fatal("the daemon was never asked to chat")
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(bodies[len(bodies)-1]), &decoded); err != nil {
		t.Fatalf("decode chat request: %v", err)
	}
	return decoded
}

func chatOptions(t *testing.T, request map[string]any) map[string]any {
	t.Helper()
	options, ok := request["options"].(map[string]any)
	if !ok {
		t.Fatalf("request carries no options: %v", request)
	}
	return options
}

func showBody(contextLength int, capabilities ...string) string {
	return mustJSON(map[string]any{
		"capabilities": capabilities,
		"details":      map[string]any{"parameter_size": "8.2B", "family": "qwen3"},
		"model_info": map[string]any{
			"general.architecture":   "qwen3",
			"qwen3.context_length":   contextLength,
			"qwen3.embedding_length": 4096,
		},
	})
}

// TestOllamaAlwaysSendsContextWindow is the load-bearing test for this
// provider: an absent num_ctx is the silent truncation it exists to prevent, so
// no resolution path may leave it out.
func TestOllamaAlwaysSendsContextWindow(t *testing.T) {
	cases := []struct {
		name          string
		configured    int
		daemonWindow  int
		daemonAnswers bool
		want          float64
	}{
		{name: "configured wins", configured: 8_000, daemonWindow: 131_072, daemonAnswers: true, want: 8_000},
		{name: "daemon answer is capped", daemonWindow: 131_072, daemonAnswers: true, want: config.DefaultContextWindow},
		{name: "small model keeps its own window", daemonWindow: 4_096, daemonAnswers: true, want: 4_096},
		{name: "no answer still sends one", want: config.DefaultContextWindow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := newOllamaUpstream(t)
			if tc.daemonAnswers {
				up.show = showBody(tc.daemonWindow, OllamaCapabilityTools)
			}
			client, err := NewClient(Config{
				Provider:      cllm.ProviderOllama,
				BaseURL:       up.server.URL,
				Model:         "qwen3:8b",
				ContextWindow: tc.configured,
			})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if _, err := client.ChatCompletionBlocking(
				context.Background(), cllm.Request{Messages: []cllm.Message{{Role: "user", Content: "hi"}}}); err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			if got := chatOptions(t, up.lastChatRequest(t))["num_ctx"]; got != tc.want {
				t.Errorf("num_ctx = %v, want %v", got, tc.want)
			}
			// The window a caller trims against and the one the daemon is told
			// to use are the same number, or the trimming is a fiction.
			if got := float64(client.ContextWindow()); got != tc.want {
				t.Errorf("ContextWindow() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestOllamaSendsItsKnobs pins the fields that are unreachable through the
// compatibility endpoint, which is the reason this adapter exists.
func TestOllamaSendsItsKnobs(t *testing.T) {
	up := newOllamaUpstream(t)
	client, err := NewClient(Config{
		Provider:      cllm.ProviderOllama,
		BaseURL:       up.server.URL,
		Model:         "qwen3:8b",
		ContextWindow: 16_000,
		MaxTokens:     512,
		KeepAlive:     "30m",
		Reasoning:     config.ReasoningMedium,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.ChatCompletionBlocking(
		context.Background(), cllm.Request{Messages: []cllm.Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	request := up.lastChatRequest(t)
	if got := request["keep_alive"]; got != "30m" {
		t.Errorf("keep_alive = %v, want 30m", got)
	}
	// Any level other than off is on: this protocol's think is a switch, and a
	// level it does not know would fail the call rather than reason less.
	if got := request["think"]; got != true {
		t.Errorf("think = %v, want true for reasoning %q", got, config.ReasoningMedium)
	}
	if got := chatOptions(t, request)["num_predict"]; got != float64(512) {
		t.Errorf("num_predict = %v, want 512", got)
	}
}

func TestOllamaReasoningOffSendsNoThink(t *testing.T) {
	up := newOllamaUpstream(t)
	client, err := NewClient(Config{
		Provider:      cllm.ProviderOllama,
		BaseURL:       up.server.URL,
		Model:         "qwen3:8b",
		ContextWindow: 16_000,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.ChatCompletionBlocking(
		context.Background(), cllm.Request{Messages: []cllm.Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	if _, present := up.lastChatRequest(t)["think"]; present {
		t.Error("a model with reasoning off should not be asked to think")
	}
}

// TestOllamaToolResultsPairByName covers the repair this protocol needs: it has
// no identifiers, so a result is matched to its call by position in the history
// and sent by name.
func TestOllamaToolResultsPairByName(t *testing.T) {
	up := newOllamaUpstream(t)
	client, err := NewClient(Config{
		Provider:      cllm.ProviderOllama,
		BaseURL:       up.server.URL,
		Model:         "qwen3:8b",
		ContextWindow: 16_000,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	history := []cllm.Message{
		{Role: "user", Content: "read both"},
		{Role: "assistant", ToolCalls: []cllm.ToolCall{
			{ID: "call_1", Name: "read_file", Arguments: `{"path":"a.go"}`},
			{ID: "call_2", Name: "list_dir", Arguments: `{"path":"."}`},
		}},
		// Answered out of order, which the agent loop is free to do.
		{Role: "tool", ToolCallID: "call_2", Content: "a.go b.go"},
		{Role: "tool", ToolCallID: "call_1", Content: "package a"},
		// An orphan: its call was trimmed or compacted away.
		{Role: "tool", ToolCallID: "call_9", Content: "stranded"},
	}
	if _, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{Messages: history}); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	var request struct {
		Messages []ollamaMessage `json:"messages"`
	}
	if err := json.Unmarshal([]byte(up.bodies["/api/chat"][0]), &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	var results []ollamaMessage
	for _, m := range request.Messages {
		if m.Role == "tool" {
			results = append(results, m)
		}
	}
	if len(results) != 2 {
		t.Fatalf("got %d tool results, want the two whose calls are still in history: %+v", len(results), results)
	}
	if results[0].ToolName != "list_dir" || results[1].ToolName != "read_file" {
		t.Errorf("results named %q and %q, want list_dir then read_file", results[0].ToolName, results[1].ToolName)
	}
	if strings.Contains(up.bodies["/api/chat"][0], "stranded") {
		t.Error("a result whose call is gone should be dropped, not sent unanswerable")
	}
}

// TestOllamaMintsIdentifiersThatDoNotRepeat keeps a long session unambiguous:
// a protocol that pairs by identifier must not meet the same one twice when it
// replays a session this adapter wrote.
func TestOllamaMintsIdentifiersThatDoNotRepeat(t *testing.T) {
	up := newOllamaUpstream(t)
	up.chat = ollamaBody(reply{toolCalls: []cllm.ToolCall{
		{Name: "read_file", Arguments: `{"path":"c.go"}`},
	}})
	client, err := NewClient(Config{
		Provider:      cllm.ProviderOllama,
		BaseURL:       up.server.URL,
		Model:         "qwen3:8b",
		ContextWindow: 16_000,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	history := []cllm.Message{
		{Role: "user", Content: "read them"},
		{Role: "assistant", ToolCalls: []cllm.ToolCall{
			{ID: "call_1", Name: "read_file", Arguments: `{"path":"a.go"}`},
			{ID: "call_2", Name: "read_file", Arguments: `{"path":"b.go"}`},
		}},
		{Role: "tool", ToolCallID: "call_1", Content: "package a"},
		{Role: "tool", ToolCallID: "call_2", Content: "package b"},
	}
	completion, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{Messages: history})
	if err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	if len(completion.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(completion.ToolCalls))
	}
	if got := completion.ToolCalls[0].ID; got != "call_3" {
		t.Errorf("minted %q, want call_3 — the identifiers must continue past the two already in history", got)
	}
}

// TestOllamaArgumentsSurviveBothDirections pins the other shape difference:
// arguments are an object here and a string in the canonical format.
func TestOllamaArgumentsSurviveBothDirections(t *testing.T) {
	up := newOllamaUpstream(t)
	up.chat = ollamaBody(reply{toolCalls: []cllm.ToolCall{
		{Name: "write_file", Arguments: `{"path":"a.go","content":"package a"}`},
	}})
	client, err := NewClient(Config{
		Provider:      cllm.ProviderOllama,
		BaseURL:       up.server.URL,
		Model:         "qwen3:8b",
		ContextWindow: 16_000,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	completion, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{Messages: []cllm.Message{
		{Role: "user", Content: "write it"},
	}})
	if err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	var arguments map[string]string
	if err := json.Unmarshal([]byte(completion.ToolCalls[0].Arguments), &arguments); err != nil {
		t.Fatalf("arguments are not a JSON object: %v", err)
	}
	if arguments["path"] != "a.go" || arguments["content"] != "package a" {
		t.Errorf("arguments = %v, want the two the model sent", arguments)
	}
}

func TestOllamaBaseURLNormalization(t *testing.T) {
	cases := []struct{ raw, want string }{
		{raw: "", want: config.DefaultOllamaBaseURL},
		{raw: "http://localhost:11434", want: "http://localhost:11434"},
		{raw: "http://localhost:11434/", want: "http://localhost:11434"},
		// The compatibility endpoint answers a different protocol; leaving the
		// suffix on would send these requests to a path that does not serve them.
		{raw: "http://localhost:11434/v1", want: "http://localhost:11434"},
		{raw: "http://localhost:11434/v1/", want: "http://localhost:11434"},
	}
	for _, tc := range cases {
		if got := ollamaBaseURL(tc.raw); got != tc.want {
			t.Errorf("ollamaBaseURL(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// TestOllamaDaemonDownFailsAtOnce keeps a local failure immediate. Backing off
// three times only delays the one sentence that helps.
func TestOllamaDaemonDownFailsAtOnce(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := closed.URL
	closed.Close()

	adapter, err := newOllamaAdapter(Config{BaseURL: url, Model: "qwen3:8b"}, 16_000)
	if err != nil {
		t.Fatalf("newOllamaAdapter: %v", err)
	}
	_, err = adapter.blocking(context.Background(), cllm.Request{Messages: []cllm.Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("expected an error when nothing is listening")
	}
	if isRetryableError(err) {
		t.Error("a daemon that is not running is not a transient failure")
	}
	if !strings.Contains(err.Error(), "ollama serve") {
		t.Errorf("error %q should say how to start the daemon", err)
	}
}

// TestOllamaMissingModelSaysHowToPull covers the other failure a local runtime
// has and a hosted one does not.
func TestOllamaMissingModelSaysHowToPull(t *testing.T) {
	up := newOllamaUpstream(t)
	up.status = http.StatusNotFound
	client, err := NewClient(Config{
		Provider:      cllm.ProviderOllama,
		BaseURL:       up.server.URL,
		Model:         "qwen3:8b",
		ContextWindow: 16_000,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.ChatCompletionBlocking(
		context.Background(), cllm.Request{Messages: []cllm.Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("expected an error for a model that is not pulled")
	}
	if !strings.Contains(err.Error(), "ollama pull qwen3:8b") {
		t.Errorf("error %q should name the command that fixes it", err)
	}
	if up.requests["/api/chat"] != 1 {
		t.Errorf("daemon saw %d requests, want 1: pulling a model is not something a retry does",
			up.requests["/api/chat"])
	}
}

// TestOllamaImagesGoAsRawBase64 pins the last shape difference: this protocol
// takes the payload alone, where the OpenAI ones take a data URL.
func TestOllamaImagesGoAsRawBase64(t *testing.T) {
	image := cllm.ContentPart{Type: cllm.ContentPartImage, MediaType: "image/png", Data: "aGVsbG8="}
	history := []cllm.Message{
		{Role: "user", Content: "look", Parts: []cllm.ContentPart{image}},
		{Role: "assistant", ToolCalls: []cllm.ToolCall{{ID: "call_1", Name: "screenshot"}}},
		{Role: "tool", ToolCallID: "call_1", Content: "a screenshot", Parts: []cllm.ContentPart{image}},
	}
	for _, vision := range []bool{true, false} {
		t.Run(map[bool]string{true: "vision on", false: "vision off"}[vision], func(t *testing.T) {
			up := newOllamaUpstream(t)
			client, err := NewClient(Config{
				Provider:      cllm.ProviderOllama,
				BaseURL:       up.server.URL,
				Model:         "qwen3:8b",
				ContextWindow: 16_000,
				Vision:        vision,
			})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if _, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{Messages: history}); err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			body := up.bodies["/api/chat"][0]
			if strings.Contains(body, "data:image/png;base64") {
				t.Error("this protocol takes raw base64, not a data URL")
			}
			carried := strings.Contains(body, `"images":["aGVsbG8="]`)
			if carried != vision {
				t.Errorf("images carried = %v with vision %v", carried, vision)
			}
			// A tool result cannot hold an image here, so one follows as its
			// own user turn — with a preamble, or it reads as the user's.
			if vision && !strings.Contains(body, imageFollowUpPreamble) {
				t.Error("an image a tool returned should follow the result with a preamble")
			}
		})
	}
}

func TestOllamaInventoryReadsTheDaemon(t *testing.T) {
	up := newOllamaUpstream(t)
	up.tags = mustJSON(map[string]any{"models": []any{
		map[string]any{
			"model": "qwen3:8b",
			"size":  5_200_000_000,
			"details": map[string]any{
				"family": "qwen3", "parameter_size": "8.2B", "quantization_level": "Q4_K_M",
			},
			"capabilities": []string{"completion", "tools"},
		},
	}})
	installed, err := OllamaInventory(context.Background(), up.server.URL)
	if err != nil {
		t.Fatalf("OllamaInventory: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("got %d models, want 1", len(installed))
	}
	got := installed[0]
	switch {
	case got.Model != "qwen3:8b":
		t.Errorf("model = %q", got.Model)
	case got.ParameterSize != "8.2B":
		t.Errorf("parameter size = %q", got.ParameterSize)
	case got.SizeBytes != 5_200_000_000:
		t.Errorf("size = %d", got.SizeBytes)
	case !got.HasCapability(OllamaCapabilityTools):
		t.Errorf("capabilities = %v, want tools among them", got.Capabilities)
	}
}

func TestOllamaShowReadsContextLengthAndCapabilities(t *testing.T) {
	up := newOllamaUpstream(t)
	up.show = showBody(40_960, OllamaCapabilityTools, OllamaCapabilityVision)
	shown, err := OllamaShow(context.Background(), up.server.URL, "qwen3:8b")
	if err != nil {
		t.Fatalf("OllamaShow: %v", err)
	}
	if shown.ContextWindow != 40_960 {
		t.Errorf("context window = %d, want 40960", shown.ContextWindow)
	}
	if !shown.HasCapability(OllamaCapabilityVision) {
		t.Errorf("capabilities = %v, want vision among them", shown.Capabilities)
	}
}

// TestModelInfoContextLengthFallsBackToAnySuffix keeps a new architecture from
// silently reporting no window at all.
func TestModelInfoContextLengthFallsBackToAnySuffix(t *testing.T) {
	info := map[string]any{
		"general.architecture":         "newarch",
		"someothername.context_length": float64(8_192),
	}
	if got := modelInfoContextLength(info); got != 8_192 {
		t.Errorf("context length = %d, want 8192 from the only key that has one", got)
	}
	if got := modelInfoContextLength(nil); got != 0 {
		t.Errorf("context length = %d, want 0 when the daemon said nothing", got)
	}
}

func TestOllamaNeedsAModel(t *testing.T) {
	_, err := NewClient(Config{Provider: cllm.ProviderOllama, BaseURL: "http://127.0.0.1:1", ContextWindow: 1})
	if err == nil {
		t.Fatal("expected an error when no model is named")
	}
}

// TestOllamaIdentifiersSurviveTrimming covers the corner the count alone does
// not: history shortens, so numbering from its length could hand out an
// identifier the session already used.
func TestOllamaIdentifiersSurviveTrimming(t *testing.T) {
	trimmed := []cllm.Message{
		// What is left after the first turns were trimmed away.
		{Role: "assistant", ToolCalls: []cllm.ToolCall{{ID: "call_7", Name: "read_file"}}},
		{Role: "tool", ToolCallID: "call_7", Content: "package a"},
	}
	if got := priorToolCalls(trimmed); got != 7 {
		t.Errorf("offset = %d, want 7 — the next identifier must clear the highest already used", got)
	}
	// An identifier another protocol minted carries no number, so the count is
	// what keeps the next one distinct.
	foreign := []cllm.Message{
		{Role: "assistant", ToolCalls: []cllm.ToolCall{
			{ID: "toolu_01ABC", Name: "read_file"},
			{ID: "toolu_02DEF", Name: "read_file"},
		}},
	}
	if got := priorToolCalls(foreign); got != 2 {
		t.Errorf("offset = %d, want 2", got)
	}
}
