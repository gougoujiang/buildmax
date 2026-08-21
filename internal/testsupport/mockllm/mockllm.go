// Package mockllm serves scripted model replies over the three wire protocols
// BuildMax speaks, so an end-to-end suite can drive a real run without a
// provider, a key, or a paid call.
//
// A scenario is a list of steps, replayed in order: one step answers one model
// call, whatever protocol asked. Nothing is inferred from the request, because
// a mock that answers what it thinks was asked stops being evidence — the run
// it produces is the mock's opinion rather than the agent's behaviour.
//
// See docs/design/end-to-end-testing.md §4.
package mockllm

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

// ToolCall is one call the scripted assistant turn makes.
type ToolCall struct {
	// ID is what the tool result will reference. Left empty, the handler names
	// it after its position so a scenario does not have to invent ids.
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// Usage is the token report for one step. Runs assert on it, so it is scripted
// rather than invented per request.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Step is one scripted model reply.
//
// A step carries several tool calls because the runtime schedules a turn's
// calls concurrently: a format with one call per turn could not express the
// case that scheduling has to leave unchanged. See
// docs/design/parallel-tool-execution.md.
type Step struct {
	Text      string     `json:"text,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     *Usage     `json:"usage,omitempty"`
	// Status and Error make the step a provider failure instead of a reply.
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Scenario is the ordered script for one run.
type Scenario struct {
	Name  string `json:"name,omitempty"`
	Steps []Step `json:"steps"`
}

// LoadScenario reads a committed scenario file.
func LoadScenario(path string) (Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, fmt.Errorf("read scenario: %w", err)
	}
	var s Scenario
	if err := json.Unmarshal(data, &s); err != nil {
		return Scenario{}, fmt.Errorf("parse scenario %s: %w", path, err)
	}
	if len(s.Steps) == 0 {
		return Scenario{}, fmt.Errorf("scenario %s has no steps", path)
	}
	return s, nil
}

// Protocol names the wire protocol a request arrived on. The values match
// config.LLMProvider* so a suite can configure a model entry from one constant.
const (
	ProtocolOpenAIChat      = "openai_compatible"
	ProtocolOpenAIResponses = "openai"
	ProtocolAnthropic       = "anthropic"
)

// Request is one model call the agent made, kept so a suite can assert on the
// history it sent as well as on what came back.
type Request struct {
	Protocol string
	Stream   bool
	Body     []byte
}

// Handler replays a scenario over HTTP.
type Handler struct {
	mu       sync.Mutex
	steps    []Step
	next     int
	requests []Request
}

// NewHandler returns a handler that replays s once.
func NewHandler(s Scenario) *Handler { return &Handler{steps: s.Steps} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "mockllm: only POST", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "mockllm: read request: "+err.Error(), http.StatusBadRequest)
		return
	}
	protocol, ok := protocolFor(r.URL.Path)
	if !ok {
		http.Error(w, fmt.Sprintf("mockllm: no protocol serves %s", r.URL.Path), http.StatusNotFound)
		return
	}
	stream := requestsStream(body)
	step, index, ok := h.take(Request{Protocol: protocol, Stream: stream, Body: body})
	if !ok {
		// Repeating the last step here would turn "the run called the model
		// more times than the scenario describes" into a passing test. The
		// status is a client error so the caller's retry loop does not spend
		// its backoff on a fault no retry can fix.
		http.Error(w, "mockllm: scenario exhausted, the run made more model calls than it scripts", http.StatusBadRequest)
		return
	}
	if step.Status != 0 {
		message := step.Error
		if message == "" {
			message = "scripted provider failure"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(step.Status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": message, "type": "mockllm"}})
		return
	}
	step = named(step, index)
	switch protocol {
	case ProtocolOpenAIChat:
		writeOpenAIChat(w, step, modelOf(body), stream)
	case ProtocolOpenAIResponses:
		writeOpenAIResponses(w, step, modelOf(body), stream)
	case ProtocolAnthropic:
		writeAnthropic(w, step, modelOf(body), stream)
	}
}

// take consumes the next step and records the call that consumed it.
func (h *Handler) take(req Request) (Step, int, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, req)
	if h.next >= len(h.steps) {
		return Step{}, 0, false
	}
	step := h.steps[h.next]
	index := h.next
	h.next++
	return step, index, true
}

// Remaining reports how many steps were never consumed. A suite fails on a
// non-zero value: a run that stopped calling the model one turn early leaves
// the final output looking plausible, and this is the only signal that says
// otherwise.
func (h *Handler) Remaining() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.steps) - h.next
}

// Requests returns the calls made so far, in order.
func (h *Handler) Requests() []Request {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Request(nil), h.requests...)
}

// Server is a handler listening on a local port.
type Server struct {
	*Handler
	listener net.Listener
	http     *http.Server
	done     chan struct{}
}

// Start serves scenario on a loopback port until Close.
func Start(scenario Scenario) (*Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("mockllm listen: %w", err)
	}
	handler := NewHandler(scenario)
	srv := &Server{Handler: handler, listener: listener, http: &http.Server{Handler: handler}, done: make(chan struct{})}
	go func() {
		defer close(srv.done)
		_ = srv.http.Serve(listener)
	}()
	return srv, nil
}

// Close stops serving.
func (s *Server) Close() {
	_ = s.http.Close()
	<-s.done
}

// URL is the server's root.
func (s *Server) URL() string { return "http://" + s.listener.Addr().String() }

// BaseURL is the api_url a model entry needs for this protocol. The two
// families disagree about where the version segment lives: the OpenAI client
// appends its path to the configured base, and the Anthropic client appends
// "v1/messages" of its own.
func (s *Server) BaseURL(protocol string) string {
	if protocol == ProtocolAnthropic {
		return s.URL()
	}
	return s.URL() + "/v1"
}

func protocolFor(path string) (string, bool) {
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		return ProtocolOpenAIChat, true
	case strings.HasSuffix(path, "/responses"):
		return ProtocolOpenAIResponses, true
	case strings.HasSuffix(path, "/messages"):
		return ProtocolAnthropic, true
	}
	return "", false
}

// named gives every tool call an id, derived from where it sits in the
// scenario so the same script always produces the same ids.
func named(step Step, index int) Step {
	calls := make([]ToolCall, len(step.ToolCalls))
	for i, call := range step.ToolCalls {
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d_%d", index+1, i+1)
		}
		calls[i] = call
	}
	step.ToolCalls = calls
	return step
}

func requestsStream(body []byte) bool {
	var req struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &req)
	return req.Stream
}

func modelOf(body []byte) string {
	var req struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &req) != nil || req.Model == "" {
		return "mockllm"
	}
	return req.Model
}

// argumentsJSON renders a call's arguments as the JSON string the OpenAI
// protocols carry. An unencodable value becomes an empty object rather than
// failing the request: the scenario is the thing under test, not the encoder.
func argumentsJSON(call ToolCall) string {
	if call.Args == nil {
		return "{}"
	}
	encoded, err := json.Marshal(call.Args)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
