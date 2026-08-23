package model

import "context"

// Task result delivery statuses.
const (
	// DeliveryPending is a report that is owed and has not been made.
	DeliveryPending = "PENDING"
	// DeliveryDelivered is a report that reached its conversation.
	DeliveryDelivered = "DELIVERED"
	// DeliveryAbandoned is a report that will not be attempted again. The run's
	// outcome is not lost with it — a task's card reads the run directly.
	DeliveryAbandoned = "ABANDONED"
)

// TaskResultDelivery is one owed report: a run that finished and a conversation
// that has not yet been told.
//
// It exists because the report is a Tier 1 turn, and a turn is a model call that
// can fail, be refused, or be interrupted by a restart. Without a record of the
// obligation, a report that does not happen simply does not happen, and nothing
// afterwards knows one was owed. What the report says is not stored: it is
// derived from the run each attempt, so a retry reports the run as it is rather
// than as it was when it finished.
type TaskResultDelivery struct {
	TaskRunID      string
	ConversationID string
	Status         string
	// Attempts counts claims, not successes. It is incremented when a delivery
	// is claimed rather than when one fails, so an attempt that dies mid-flight
	// still counts against the cap.
	Attempts  int
	LastError *string
	// NextAttemptAt is both the backoff and the lease: claiming pushes it out,
	// so a second sweeper does not pick up a delivery already in flight.
	NextAttemptAt int64
	CreatedAt     int64
}

// TaskResultDeliveryStore persists owed reports.
type TaskResultDeliveryStore interface {
	// EnqueueTaskResultDelivery records that a run's outcome is owed to a
	// conversation. It is idempotent per run: a run reported twice — by its
	// worker and then by the reaper that gave up on it — owes one report.
	EnqueueTaskResultDelivery(ctx context.Context, taskRunID, conversationID string, now int64) error
	// ListDueTaskResultDeliveries returns pending reports whose next attempt is
	// due, oldest first.
	ListDueTaskResultDeliveries(ctx context.Context, now int64, limit int) ([]TaskResultDelivery, error)
	// ClaimTaskResultDelivery takes one pending delivery that is due, counting
	// the attempt and pushing its next one to nextAttemptAt. It returns nil
	// when the delivery is not pending, not due, or was claimed by someone
	// else — which is what keeps one run from being reported twice.
	ClaimTaskResultDelivery(ctx context.Context, taskRunID string, now, nextAttemptAt int64) (*TaskResultDelivery, error)
	// FinishTaskResultDelivery closes a delivery as DELIVERED or ABANDONED.
	FinishTaskResultDelivery(ctx context.Context, taskRunID, status string, lastError *string) error
	// RecordTaskResultDeliveryFailure keeps a delivery pending, records why the
	// last attempt did not succeed, and brings its next attempt forward. The
	// claim pushed that time out far enough to protect a turn still running;
	// an attempt that has already failed no longer needs protecting.
	RecordTaskResultDeliveryFailure(ctx context.Context, taskRunID, lastError string, nextAttemptAt int64) error
}
