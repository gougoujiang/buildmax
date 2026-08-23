package worker

// Fixtures copied from the root package rather than imported: a test helper
// shared across a package boundary makes the test boundary softer than the
// code's, which is what splitting these packages was for.

import (
	"context"
	"testing"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

const (
	llmTestSecret = "test-llm-secret"
	llmTestUser   = "u_llm"
	llmTestTeam   = "tm_llm"
)

// llmStubClient answers every call the same way.
func llmTestService(t *testing.T, client cllm.LLMClient, quota llmgateway.QuotaChecker) *llmgateway.Service {
	t.Helper()

	fast := llmgateway.Target{
		ID:            "mt_fast",
		Name:          "Fast",
		ProviderType:  llmgateway.ProviderOpenAICompatible,
		Endpoint:      "https://SECRET-ENDPOINT.internal/v1",
		CredentialRef: "SECRET-CREDENTIAL",
		UpstreamModel: "SECRET-UPSTREAM-MODEL",
		Capabilities:  llmgateway.NewCapabilitySet(llmgateway.BaselineCapabilities()...),
		Enabled:       true,
	}
	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{fast})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	policies, err := llmgateway.NewStaticPolicySource(llmgateway.TeamPolicy{
		DefaultAlias: "default",
		Aliases:      map[string]string{"default": "mt_fast"},
	}, catalog.IDs())
	if err != nil {
		t.Fatalf("NewStaticPolicySource: %v", err)
	}
	return &llmgateway.Service{
		Router: &llmgateway.Router{
			Resolver: &llmgateway.Resolver{Catalog: catalog, Policies: policies},
			Factory: func(context.Context, llmgateway.Target) (cllm.LLMClient, error) {
				return client, nil
			},
		},
		Ledger: &llmStubLedger{},
		Quota:  quota,
	}
}

// llmStubClient answers every call the same way.
type llmStubClient struct {
	content string
	deltas  []string
	usage   cllm.Usage
	err     error
	// gotProfile is what the gateway passed the provider client, so a test can
	// check that a worker's stated intent survived the route.
	gotProfile cllm.CallProfile
}

func (c *llmStubClient) ChatCompletionBlocking(_ context.Context, req cllm.Request) (cllm.Completion, error) {
	c.gotProfile = req.Profile
	if c.err != nil {
		return cllm.Completion{}, c.err
	}
	return cllm.Completion{Content: c.content, Usage: c.usage}, nil
}
func (c *llmStubClient) ChatCompletionStreaming(_ context.Context, req cllm.Request, onDelta func(string)) (cllm.Completion, error) {
	c.gotProfile = req.Profile
	for _, delta := range c.deltas {
		onDelta(delta)
	}
	if c.err != nil {
		return cllm.Completion{}, c.err
	}
	return cllm.Completion{Content: c.content, Usage: c.usage}, nil
}
func (c *llmStubClient) ContextWindow() int { return 0 }

// llmStubLedger accepts every write and keeps the last one so a test can check
// what a call was attributed to.
type llmStubLedger struct {
	opened  int
	last    model.LLMCall
	calls   []model.LLMCall
	listErr error
}

func (l *llmStubLedger) OpenLLMCall(_ context.Context, call *model.LLMCall) (*model.LLMCall, error) {
	l.opened++
	stored := *call
	stored.ID = "lc_stub"
	l.last = stored
	return &stored, nil
}
func (l *llmStubLedger) CompleteLLMCall(context.Context, string, model.LLMCallOutcome) error {
	return nil
}
func (l *llmStubLedger) GetLLMCall(context.Context, string) (*model.LLMCall, error) { return nil, nil }

func (l *llmStubLedger) GetLLMCallByClientID(context.Context, string, string) (*model.LLMCall, error) {
	return nil, nil
}
func (l *llmStubLedger) ListLLMCallsByTaskRun(_ context.Context, teamID, taskRunID string) ([]model.LLMCall, error) {
	if l.listErr != nil {
		return nil, l.listErr
	}
	var out []model.LLMCall
	for _, call := range l.calls {
		if call.TeamID == teamID && call.TaskRunID != nil && *call.TaskRunID == taskRunID {
			out = append(out, call)
		}
	}
	return out, nil
}
