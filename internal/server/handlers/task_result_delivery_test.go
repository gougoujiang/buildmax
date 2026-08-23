package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/util"
)

// countingLLMClient answers, or refuses, and counts how many turns reached it.
type countingLLMClient struct {
	mu    sync.Mutex
	calls int
	// failUntil makes the first n completions fail, so a test can watch a
	// report be retried rather than lost.
	failUntil int
}

// The refusal names which call it was, so a test can tell one recorded failure
// from the next rather than watching a field that is already non-nil.
func (c *countingLLMClient) next() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls <= c.failUntil {
		return fmt.Errorf("the model is unavailable (call %d)", c.calls)
	}
	return nil
}

func (c *countingLLMClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *countingLLMClient) ChatCompletionBlocking(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (llm.Completion, error) {
	if err := c.next(); err != nil {
		return llm.Completion{}, err
	}
	return llm.Completion{Content: "reported"}, nil
}

func (c *countingLLMClient) ChatCompletionStreaming(ctx context.Context, messages []llm.Message, tools []llm.ToolDef, onDelta func(string)) (llm.Completion, error) {
	if err := c.next(); err != nil {
		return llm.Completion{}, err
	}
	onDelta("reported")
	return llm.Completion{Content: "reported"}, nil
}

func (c *countingLLMClient) ContextWindow() int { return 0 }

type deliveryFixture struct {
	handler    *Handler
	deliveries *mock.MockTaskResultDeliveryStore
	messages   *mock.MockConversationMessageStore
	client     *countingLLMClient
}

func newDeliveryFixture(t *testing.T, failUntil int) deliveryFixture {
	t.Helper()
	deliveries := &mock.MockTaskResultDeliveryStore{}
	messages := &mock.MockConversationMessageStore{}
	client := &countingLLMClient{failUntil: failUntil}
	h := NewHandler(Config{
		JWTSecret:                wsTestSecret,
		ConversationStore:        &mock.MockConversationStore{Conversations: []model.Conversation{{ID: "conv-1", TeamID: "tm_shared"}}},
		ConversationMessageStore: messages,
		ConversationLLMClient:    client,
		TaskResultDeliveries:     deliveries,
		TaskRunStore: &mock.MockTaskRunStore{
			Runs: []model.TaskRun{{
				ID: "tr_1", TaskID: "tk_1", Status: string(model.RunStatusSucceeded),
				Output: util.Ptr("the analysis found three problems"), CreatedAt: time.Unix(1, 0).UTC(),
			}},
			TaskList: []model.Task{{ID: "tk_1", ConversationID: "conv-1", TeamID: "tm_shared", CreatedBy: "u1"}},
		},
	})
	return deliveryFixture{handler: h, deliveries: deliveries, messages: messages, client: client}
}

// waitFor polls until the delivery satisfies want. A report is made on the turn
// queue's goroutine, so every assertion about one has to wait for it to settle.
func (f deliveryFixture) waitFor(t *testing.T, desc string, want func(*model.TaskResultDelivery) bool) *model.TaskResultDelivery {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		d := f.deliveries.Get("tr_1")
		if d != nil && want(d) {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatalf("delivery = %+v, want %s", d, desc)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f deliveryFixture) waitForStatus(t *testing.T, want string) *model.TaskResultDelivery {
	t.Helper()
	return f.waitFor(t, "status "+want, func(d *model.TaskResultDelivery) bool { return d.Status == want })
}

// waitForFailedAttempt waits until the nth model call's refusal has been
// recorded, which is the only moment the delivery is due again.
func (f deliveryFixture) waitForFailedAttempt(t *testing.T, n int) *model.TaskResultDelivery {
	t.Helper()
	want := fmt.Sprintf("(call %d)", n)
	return f.waitFor(t, "call "+strconv.Itoa(n)+" to be recorded as failed", func(d *model.TaskResultDelivery) bool {
		if d.Status == model.DeliveryAbandoned {
			return true
		}
		return d.LastError != nil && strings.Contains(*d.LastError, want)
	})
}

// A report that succeeds is recorded as owed and then closed. The record is what
// makes it possible to know one was owed at all.
func TestTaskResultDeliveryIsRecordedAndClosed(t *testing.T) {
	f := newDeliveryFixture(t, 0)

	f.handler.reportTaskRunTerminal(context.Background(), terminalInfo())

	d := f.waitForStatus(t, model.DeliveryDelivered)
	if d.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", d.Attempts)
	}
}

// This is the case the record exists for: the run finished, the report never
// happened, and nothing in memory remembers it was owed. A sweep does.
func TestASweepReportsARunThatWasNeverReported(t *testing.T) {
	f := newDeliveryFixture(t, 0)
	// What a restart leaves behind: the obligation, and nothing else.
	if err := f.deliveries.EnqueueTaskResultDelivery(context.Background(), "tr_1", "conv-1", time.Now().UTC().Add(-60*time.Second)); err != nil {
		t.Fatal(err)
	}

	f.handler.SweepTaskResultDeliveries(context.Background(), time.Now())

	f.waitForStatus(t, model.DeliveryDelivered)
	msgs, _ := f.messages.ListMessages(context.Background(), "conv-1")
	if len(msgs) < 2 {
		t.Fatalf("stored %d messages, want the report and its reply", len(msgs))
	}
}

// A failed turn leaves the report owed, with the reason, and a later sweep
// makes it. It used to be logged and forgotten.
func TestAFailedReportIsRetriedRatherThanLost(t *testing.T) {
	f := newDeliveryFixture(t, 1)

	f.handler.reportTaskRunTerminal(context.Background(), terminalInfo())

	// Still owed after the first attempt, and it says why.
	d := f.waitForFailedAttempt(t, 1)
	if d.Status != model.DeliveryPending {
		t.Fatalf("status = %q, want the report still owed", d.Status)
	}
	if d.LastError == nil || *d.LastError == "" {
		t.Fatalf("delivery = %+v, want the failure recorded", d)
	}

	// The backoff put the next attempt in the future; a sweep at that time
	// makes it, and this time the model answers.
	f.handler.SweepTaskResultDeliveries(context.Background(), time.Now().Add(2*deliveryBackoff))
	f.waitForStatus(t, model.DeliveryDelivered)
	if f.client.count() != 2 {
		t.Errorf("model calls = %d, want the failed attempt and the retry", f.client.count())
	}
}

// The claim is what keeps a run from being reported twice when the terminal
// callback and a sweep reach the same delivery.
func TestAClaimedReportIsNotReportedAgain(t *testing.T) {
	f := newDeliveryFixture(t, 0)

	f.handler.reportTaskRunTerminal(context.Background(), terminalInfo())
	f.waitForStatus(t, model.DeliveryDelivered)
	before := f.client.count()

	// A sweep that runs afterwards finds nothing due: the delivery is closed.
	f.handler.SweepTaskResultDeliveries(context.Background(), time.Now().Add(time.Hour))
	if f.client.count() != before {
		t.Errorf("model calls = %d, want no second report", f.client.count())
	}
}

// A report nothing will fix is given up on, and the giving up is recorded.
func TestAReportIsAbandonedAfterItsAttemptsRunOut(t *testing.T) {
	f := newDeliveryFixture(t, deliveryMaxAttempts+5)

	f.handler.reportTaskRunTerminal(context.Background(), terminalInfo())
	for attempt := 1; attempt <= deliveryMaxAttempts+1; attempt++ {
		if f.waitForFailedAttempt(t, attempt).Status == model.DeliveryAbandoned {
			break
		}
		f.handler.SweepTaskResultDeliveries(context.Background(), time.Now().Add(time.Duration(attempt)*2*deliveryBackoff))
	}

	d := f.waitForStatus(t, model.DeliveryAbandoned)
	if d.LastError == nil {
		t.Error("giving up should say why")
	}
	if d.Attempts > deliveryMaxAttempts+1 {
		t.Errorf("attempts = %d, want the cap to hold", d.Attempts)
	}
	// The cap bounds model calls, not just rows: the abandoning attempt makes none.
	if f.client.count() != deliveryMaxAttempts {
		t.Errorf("model calls = %d, want %d", f.client.count(), deliveryMaxAttempts)
	}
}

// A deployment with no delivery store still reports; it just cannot retry.
func TestReportingWorksWithoutADeliveryStore(t *testing.T) {
	messages := &mock.MockConversationMessageStore{}
	h := NewHandler(Config{
		JWTSecret:                wsTestSecret,
		ConversationStore:        &mock.MockConversationStore{},
		ConversationMessageStore: messages,
		ConversationLLMClient:    &countingLLMClient{},
	})

	h.reportTaskRunTerminal(context.Background(), terminalInfo())

	waitForMessages(t, messages, 2)
}
