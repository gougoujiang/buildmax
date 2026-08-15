package llmgateway_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

// scriptedClient returns a fixed reply, recording what it was asked.
type scriptedClient struct {
	content   string
	toolCalls []cllm.ToolCall
	usage     cllm.Usage
	err       error

	gotMessages []cllm.Message
	gotTools    []cllm.ToolDef
}

func (c *scriptedClient) ChatCompletionBlocking(_ context.Context, messages []cllm.Message, tools []cllm.ToolDef) (string, []cllm.ToolCall, cllm.Usage, error) {
	c.gotMessages = messages
	c.gotTools = tools
	if c.err != nil {
		return "", nil, cllm.Usage{}, c.err
	}
	return c.content, c.toolCalls, c.usage, nil
}

func (c *scriptedClient) ChatCompletionStreaming(context.Context, []cllm.Message, []cllm.ToolDef, func(string)) (string, []cllm.ToolCall, cllm.Usage, error) {
	return "", nil, cllm.Usage{}, errors.New("not used")
}

func (c *scriptedClient) ContextWindow() int { return 0 }

// fakeLedger records calls in memory.
type fakeLedger struct {
	mu       sync.Mutex
	opened   []model.LLMCall
	outcomes map[string]model.LLMCallOutcome
	openErr  error
	closeErr error
	nextID   int
}

func newFakeLedger() *fakeLedger {
	return &fakeLedger{outcomes: map[string]model.LLMCallOutcome{}}
}

func (l *fakeLedger) OpenLLMCall(_ context.Context, call *model.LLMCall) (*model.LLMCall, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.openErr != nil {
		return nil, l.openErr
	}
	l.nextID++
	stored := *call
	stored.LLMCallID = "lc_test" + string(rune('0'+l.nextID))
	l.opened = append(l.opened, stored)
	return &stored, nil
}

func (l *fakeLedger) CompleteLLMCall(_ context.Context, llmCallID string, outcome model.LLMCallOutcome) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closeErr != nil {
		return l.closeErr
	}
	l.outcomes[llmCallID] = outcome
	return nil
}

func (l *fakeLedger) GetLLMCall(context.Context, string) (*model.LLMCall, error) { return nil, nil }

func (l *fakeLedger) GetLLMCallByClientID(context.Context, string, string) (*model.LLMCall, error) {
	return nil, nil
}

func (l *fakeLedger) only(t *testing.T) (model.LLMCall, model.LLMCallOutcome) {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.opened) != 1 {
		t.Fatalf("ledger opened %d calls, want 1", len(l.opened))
	}
	call := l.opened[0]
	return call, l.outcomes[call.LLMCallID]
}

// denyQuota refuses every team.
type denyQuota struct{ reason string }

func (q denyQuota) Check(context.Context, string, int, int) (bool, string) { return false, q.reason }

// allowQuota accepts every team and records that it was asked.
type allowQuota struct{ calls int }

func (q *allowQuota) Check(context.Context, string, int, int) (bool, string) {
	q.calls++
	return true, ""
}

func serviceWith(t *testing.T, client cllm.LLMClient, ledger model.LLMCallStore, quota llmgateway.QuotaChecker) *llmgateway.Service {
	t.Helper()
	router := &llmgateway.Router{
		Resolver: testResolver(t),
		Factory: func(context.Context, llmgateway.Target) (cllm.LLMClient, error) {
			return client, nil
		},
	}
	fixed := time.Unix(1_700_000_000, 0)
	return &llmgateway.Service{
		Router: router,
		Ledger: ledger,
		Quota:  quota,
		Now:    func() time.Time { return fixed },
	}
}

func userRequest() llmgateway.CompleteRequest {
	userID := "u_one"
	return llmgateway.CompleteRequest{
		TeamID:   "tm_one",
		UserID:   &userID,
		Surface:  model.LLMCallSurfaceCLI,
		Messages: []cllm.Message{{Role: "user", Content: "hello"}},
	}
}

func TestCompleteRecordsASuccessfulCall(t *testing.T) {
	client := &scriptedClient{
		content: "hi there",
		usage:   cllm.Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
	}
	ledger := newFakeLedger()
	svc := serviceWith(t, client, ledger, &allowQuota{})

	result, err := svc.Complete(context.Background(), userRequest())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result.Content != "hi there" {
		t.Errorf("Content = %q, want %q", result.Content, "hi there")
	}
	if result.Alias != "default" {
		t.Errorf("Alias = %q, want the team default", result.Alias)
	}
	if !result.UsageReported || result.Usage.TotalTokens != 14 {
		t.Errorf("usage = %+v, reported=%v", result.Usage, result.UsageReported)
	}

	call, outcome := ledger.only(t)
	if call.TeamID != "tm_one" || call.UserID == nil || *call.UserID != "u_one" {
		t.Errorf("ledger identity = team %q user %v", call.TeamID, call.UserID)
	}
	if call.TargetID != "mt_fast" || call.UpstreamModel != "vendor/fast-1" {
		t.Errorf("ledger model = %q / %q", call.TargetID, call.UpstreamModel)
	}
	if call.Status != model.LLMCallStatusAccepted {
		t.Errorf("opened status = %q, want %q", call.Status, model.LLMCallStatusAccepted)
	}
	if outcome.Status != model.LLMCallStatusSucceeded {
		t.Errorf("outcome status = %q, want %q", outcome.Status, model.LLMCallStatusSucceeded)
	}
	if outcome.Usage == nil || outcome.Usage.TotalTokens != 14 {
		t.Fatalf("outcome usage = %+v", outcome.Usage)
	}
	if outcome.Usage.Source != model.LLMUsageSourceReported {
		t.Errorf("usage source = %q, want %q", outcome.Usage.Source, model.LLMUsageSourceReported)
	}
	if outcome.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", outcome.Attempts)
	}
}

// TestCompleteLeavesUsageUnavailable covers a provider that reports no counts.
// Zero tokens and unknown tokens are different facts.
func TestCompleteLeavesUsageUnavailable(t *testing.T) {
	ledger := newFakeLedger()
	svc := serviceWith(t, &scriptedClient{content: "hi"}, ledger, nil)

	result, err := svc.Complete(context.Background(), userRequest())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result.UsageReported {
		t.Error("usage reported for a provider that sent none")
	}
	_, outcome := ledger.only(t)
	if outcome.Usage != nil {
		t.Errorf("outcome usage = %+v, want nil", outcome.Usage)
	}
}

func TestCompleteRecordsAFailedCall(t *testing.T) {
	boom := errors.New("provider said no: account acct_12345 over limit")
	ledger := newFakeLedger()
	svc := serviceWith(t, &scriptedClient{err: boom}, ledger, nil)

	_, err := svc.Complete(context.Background(), userRequest())
	if !errors.Is(err, llmgateway.ErrUpstream) {
		t.Fatalf("want ErrUpstream, got %v", err)
	}

	_, outcome := ledger.only(t)
	if outcome.Status != model.LLMCallStatusFailed {
		t.Errorf("outcome status = %q, want %q", outcome.Status, model.LLMCallStatusFailed)
	}
	if outcome.ErrorClass == nil || *outcome.ErrorClass != llmgateway.ErrorClassUpstream {
		t.Errorf("error class = %v, want %q", outcome.ErrorClass, llmgateway.ErrorClassUpstream)
	}
	// The ledger keeps a classification, never the provider's own words.
	if outcome.ErrorClass != nil && *outcome.ErrorClass == boom.Error() {
		t.Error("the ledger stored the provider error body")
	}
}

func TestCompleteRefusesOverQuota(t *testing.T) {
	ledger := newFakeLedger()
	svc := serviceWith(t, &scriptedClient{content: "hi"}, ledger, denyQuota{reason: "quota exceeded: token limit"})

	_, err := svc.Complete(context.Background(), userRequest())
	if !errors.Is(err, llmgateway.ErrQuotaExceeded) {
		t.Fatalf("want ErrQuotaExceeded, got %v", err)
	}
	var quotaErr *llmgateway.QuotaError
	if !errors.As(err, &quotaErr) || quotaErr.Reason == "" {
		t.Errorf("want a QuotaError with a reason, got %v", err)
	}
	// A refused call is not a call: nothing is opened in the ledger.
	if len(ledger.opened) != 0 {
		t.Errorf("ledger opened %d calls for a refused request", len(ledger.opened))
	}
}

func TestCompleteChecksQuotaAfterResolution(t *testing.T) {
	quota := &allowQuota{}
	ledger := newFakeLedger()
	svc := serviceWith(t, &scriptedClient{content: "hi"}, ledger, quota)

	req := userRequest()
	req.Alias = "reasoning"
	if _, err := svc.Complete(context.Background(), req); !errors.Is(err, llmgateway.ErrUnknownAlias) {
		t.Fatalf("want ErrUnknownAlias, got %v", err)
	}
	// An unusable alias is rejected before the quota query runs.
	if quota.calls != 0 {
		t.Errorf("quota consulted %d times for an unknown alias, want 0", quota.calls)
	}
}

func TestCompleteRequiresToolCapability(t *testing.T) {
	ledger := newFakeLedger()
	svc := serviceWith(t, &scriptedClient{content: "hi"}, ledger, nil)

	req := userRequest()
	req.Alias = "deep" // declares text_chat only
	req.Tools = []cllm.ToolDef{{Name: "read_file"}}

	_, err := svc.Complete(context.Background(), req)
	if !errors.Is(err, llmgateway.ErrCapabilityUnsupported) {
		t.Fatalf("want ErrCapabilityUnsupported, got %v", err)
	}
	if len(ledger.opened) != 0 {
		t.Error("a capability rejection opened a ledger record")
	}
}

func TestCompleteValidatesRequest(t *testing.T) {
	svc := serviceWith(t, &scriptedClient{}, newFakeLedger(), nil)

	tests := []struct {
		name string
		req  llmgateway.CompleteRequest
		want error
	}{
		{name: "no team", req: llmgateway.CompleteRequest{Messages: []cllm.Message{{Role: "user"}}}, want: llmgateway.ErrTeamRequired},
		{name: "no messages", req: llmgateway.CompleteRequest{TeamID: "tm_one"}, want: llmgateway.ErrMessagesRequired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Complete(context.Background(), tc.req); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
}

func TestCompleteNotConfigured(t *testing.T) {
	var nilService *llmgateway.Service
	if _, err := nilService.Complete(context.Background(), userRequest()); !errors.Is(err, llmgateway.ErrCatalogNotConfigured) {
		t.Errorf("nil service: want ErrCatalogNotConfigured, got %v", err)
	}

	noLedger := serviceWith(t, &scriptedClient{}, nil, nil)
	if _, err := noLedger.Complete(context.Background(), userRequest()); !errors.Is(err, llmgateway.ErrLedgerNotConfigured) {
		t.Errorf("want ErrLedgerNotConfigured, got %v", err)
	}
}

// TestCompleteSurvivesLedgerCloseFailure records the deliberate asymmetry: a
// call that cannot be opened is refused, but one that cannot be closed still
// returns its result, leaving an ACCEPTED row as the reconciliation signal.
func TestCompleteSurvivesLedgerCloseFailure(t *testing.T) {
	ledger := newFakeLedger()
	ledger.closeErr = errors.New("ledger unavailable")
	svc := serviceWith(t, &scriptedClient{content: "hi"}, ledger, nil)

	result, err := svc.Complete(context.Background(), userRequest())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result.Content != "hi" {
		t.Errorf("Content = %q, want %q", result.Content, "hi")
	}

	openFails := newFakeLedger()
	openFails.openErr = errors.New("ledger unavailable")
	refusing := serviceWith(t, &scriptedClient{content: "hi"}, openFails, nil)
	if _, err := refusing.Complete(context.Background(), userRequest()); err == nil {
		t.Error("a call ran without an open ledger record")
	}
}

func TestCompletePassesMessagesAndToolsThrough(t *testing.T) {
	client := &scriptedClient{content: "ok"}
	svc := serviceWith(t, client, newFakeLedger(), nil)

	req := userRequest()
	req.Messages = append(req.Messages, cllm.Message{Role: "assistant", Content: "prior"})
	req.Tools = []cllm.ToolDef{{Name: "read_file", Description: "reads"}}

	if _, err := svc.Complete(context.Background(), req); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(client.gotMessages) != 2 {
		t.Errorf("client got %d messages, want 2", len(client.gotMessages))
	}
	if len(client.gotTools) != 1 || client.gotTools[0].Name != "read_file" {
		t.Errorf("client got tools %+v", client.gotTools)
	}
}

func TestModelsDelegatesToTheRouter(t *testing.T) {
	svc := serviceWith(t, &scriptedClient{}, newFakeLedger(), nil)

	models, err := svc.Models(context.Background(), "tm_one")
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("Models returned %d, want 2", len(models))
	}
	if _, err := svc.Models(context.Background(), "tm_unknown"); !errors.Is(err, llmgateway.ErrTeamNotAuthorized) {
		t.Errorf("want ErrTeamNotAuthorized, got %v", err)
	}
}

func TestErrorClassFor(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{err: nil, want: ""},
		{err: llmgateway.ErrTeamRequired, want: llmgateway.ErrorClassTeamRequired},
		{err: llmgateway.ErrTeamNotAuthorized, want: llmgateway.ErrorClassTeamNotAuthorized},
		{err: llmgateway.ErrUnknownAlias, want: llmgateway.ErrorClassUnknownAlias},
		{err: llmgateway.ErrNoDefaultAlias, want: llmgateway.ErrorClassUnknownAlias},
		{err: llmgateway.ErrTargetNotFound, want: llmgateway.ErrorClassTargetNotFound},
		{err: llmgateway.ErrTargetDisabled, want: llmgateway.ErrorClassTargetDisabled},
		{err: &llmgateway.CapabilityError{Alias: "deep"}, want: llmgateway.ErrorClassCapability},
		{err: &llmgateway.QuotaError{Reason: "over"}, want: llmgateway.ErrorClassQuotaExceeded},
		{err: llmgateway.ErrMessagesRequired, want: llmgateway.ErrorClassInvalidRequest},
		{err: llmgateway.ErrCatalogNotConfigured, want: llmgateway.ErrorClassNotConfigured},
		{err: llmgateway.ErrFactoryNotConfigured, want: llmgateway.ErrorClassNotConfigured},
		{err: llmgateway.ErrLedgerNotConfigured, want: llmgateway.ErrorClassNotConfigured},
		{err: context.Canceled, want: llmgateway.ErrorClassCanceled},
		{err: context.DeadlineExceeded, want: llmgateway.ErrorClassCanceled},
		{err: llmgateway.ErrUpstream, want: llmgateway.ErrorClassUpstream},
		// Anything unrecognized is our problem until someone classifies it.
		{err: errors.New("something new"), want: llmgateway.ErrorClassInternal},
	}
	for _, tc := range tests {
		if got := llmgateway.ErrorClassFor(tc.err); got != tc.want {
			t.Errorf("ErrorClassFor(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
