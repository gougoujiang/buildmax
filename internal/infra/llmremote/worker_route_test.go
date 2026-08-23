package llmremote_test

import (
	"context"
	"strings"
	"testing"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/infra/llmremote"
)

// TestWorkerModeCallsTheRunRoute covers the routing decision that lets a task
// run reach the gateway at all. The team route authenticates a person; a worker
// has no person to be, so it calls as the run it was dispatched for and lets the
// server derive the rest from the run token.
func TestWorkerModeCallsTheRunRoute(t *testing.T) {
	gateway := newFakeGateway(t)
	gateway.body = `{"llm_call_id":"lc_1","model":"default","content":"hi"}`

	client := gateway.client(llmremote.Config{Token: "run-token", TaskRunID: "r_1", Model: "Fast"})
	if _, err := client.ChatCompletionBlocking(context.Background(),
		cllm.Request{Messages: []cllm.Message{{Role: "user", Content: "hello"}}}); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}

	if gateway.gotPath != "/api/worker/task-runs/r_1/llm/completions" {
		t.Errorf("path = %q, want the worker route", gateway.gotPath)
	}
	if gateway.gotAuth != "Bearer run-token" {
		t.Errorf("authorization = %q", gateway.gotAuth)
	}
	// The body must not name a team. A worker that could state one would be
	// asserting an identity the server is supposed to derive.
	if strings.Contains(string(gateway.gotRaw), "team") {
		t.Errorf("worker request body mentions a team: %s", gateway.gotRaw)
	}
}

func TestWorkerModeStreams(t *testing.T) {
	gateway := newFakeGateway(t)
	gateway.body = "event: delta\ndata: {\"content\":\"par\"}\n\n" +
		"event: result\ndata: {\"content\":\"partial\"}\n\n"

	client := gateway.client(llmremote.Config{Token: "run-token", TaskRunID: "r_2"})
	var deltas []string
	completion, err := client.ChatCompletionStreaming(context.Background(),
		cllm.Request{Messages: []cllm.Message{{Role: "user", Content: "hi"}}},
		func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("ChatCompletionStreaming: %v", err)
	}
	if gateway.gotPath != "/api/worker/task-runs/r_2/llm/completions" {
		t.Errorf("path = %q, want the worker route", gateway.gotPath)
	}
	if completion.Content != "partial" || len(deltas) != 1 {
		t.Errorf("content = %q, deltas = %v", completion.Content, deltas)
	}
}

// A client with no server URL cannot address anything, and says so rather than
// building a request against an empty host.
func TestClientWithNoServerMakesNoCall(t *testing.T) {
	client := llmremote.NewClient(llmremote.Config{Token: "tok"})
	if _, err := client.ChatCompletionBlocking(context.Background(),
		cllm.Request{Messages: []cllm.Message{{Role: "user"}}}); err == nil {
		t.Fatal("a client with no server URL made a call")
	}
}

// TestWorkerModeRefusesModelDiscovery records that choosing a model is not a
// task run's decision. It is told which model to use at dispatch.
func TestWorkerModeRefusesModelDiscovery(t *testing.T) {
	gateway := newFakeGateway(t)

	client := gateway.client(llmremote.Config{Token: "run-token", TaskRunID: "r_1"})
	if _, err := client.Models(context.Background()); err == nil {
		t.Fatal("a worker-mode client listed models")
	}
	if gateway.requests != 0 {
		t.Errorf("the gateway was reached %d times", gateway.requests)
	}
}
