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

	client := gateway.client(llmremote.Config{Token: "run-token", TaskRunID: "r_1", Alias: "default"})
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

// TestTeamAndTaskRunAreMutuallyExclusive keeps a confused caller from silently
// picking a route. Both fields set means the caller has not decided which
// identity it is asserting, and guessing for it would hide the mistake until it
// showed up as somebody else's ledger entry.
func TestTeamAndTaskRunAreMutuallyExclusive(t *testing.T) {
	gateway := newFakeGateway(t)

	client := gateway.client(llmremote.Config{Token: "tok", TeamID: "tm_one", TaskRunID: "r_1"})
	_, err := client.ChatCompletionBlocking(context.Background(),
		cllm.Request{Messages: []cllm.Message{{Role: "user"}}})
	if err == nil {
		t.Fatal("a client holding both a team and a task run made a call")
	}
	if !strings.Contains(err.Error(), "only call as one") {
		t.Errorf("error %q does not explain the conflict", err)
	}
	if gateway.requests != 0 {
		t.Errorf("the gateway was reached %d times; the check must be client-side", gateway.requests)
	}
}

func TestClientWithNoIdentityMakesNoCall(t *testing.T) {
	gateway := newFakeGateway(t)

	client := llmremote.NewClient(llmremote.Config{ServerURL: gateway.server.URL, Token: "tok"})
	if _, err := client.ChatCompletionBlocking(context.Background(),
		cllm.Request{Messages: []cllm.Message{{Role: "user"}}}); err == nil {
		t.Fatal("a client with neither a team nor a task run made a call")
	}
	if gateway.requests != 0 {
		t.Errorf("the gateway was reached %d times", gateway.requests)
	}
}

// TestWorkerModeRefusesModelDiscovery records that choosing a model is not a
// task run's decision. It is told which alias to use at dispatch; browsing the
// team's catalog is a team capability.
func TestWorkerModeRefusesModelDiscovery(t *testing.T) {
	gateway := newFakeGateway(t)

	client := gateway.client(llmremote.Config{Token: "run-token", TaskRunID: "r_1"})
	if _, err := client.Models(context.Background()); err == nil {
		t.Fatal("a worker-mode client listed team models")
	}
	if gateway.requests != 0 {
		t.Errorf("the gateway was reached %d times", gateway.requests)
	}
}
