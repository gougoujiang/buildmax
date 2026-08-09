package retry

import (
	"errors"
	"testing"
	"time"
)

var errTemp = errors.New("temporary error")

func TestDo_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	err := Do(func() error {
		calls++
		return nil
	}, 3, time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls != 1 {
		t.Errorf("fn called %d times, want 1", calls)
	}
}

func TestDo_SuccessAfterRetries(t *testing.T) {
	calls := 0
	err := Do(func() error {
		calls++
		if calls < 3 {
			return errTemp
		}
		return nil
	}, 5, time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil after 3rd attempt, got %v", err)
	}
	if calls != 3 {
		t.Errorf("fn called %d times, want 3", calls)
	}
}

func TestDo_AllAttemptsFail(t *testing.T) {
	calls := 0
	err := Do(func() error {
		calls++
		return errTemp
	}, 3, time.Millisecond)
	if err == nil {
		t.Fatal("expected error after all attempts fail")
	}
	if calls != 3 {
		t.Errorf("fn called %d times, want 3", calls)
	}
}

func TestDo_ReturnsLastError(t *testing.T) {
	sentinel := errors.New("final error")
	count := 0
	err := Do(func() error {
		count++
		if count == 2 {
			return sentinel
		}
		return errors.New("earlier error")
	}, 2, time.Millisecond)
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want sentinel", err)
	}
}

func TestDo_ExponentialBackoff(t *testing.T) {
	// Verify that total elapsed time reflects exponential delays.
	// With backoff=10ms and 3 attempts: sleep 10ms then 20ms between attempts.
	// Total wait ≥ 30ms (2 sleeps).
	calls := 0
	start := time.Now()
	_ = Do(func() error {
		calls++
		return errTemp
	}, 3, 10*time.Millisecond)
	elapsed := time.Since(start)
	if elapsed < 25*time.Millisecond {
		t.Errorf("elapsed %v < 25ms; backoff may not be applied", elapsed)
	}
}

func TestDo_ZeroAttempts(t *testing.T) {
	calls := 0
	_ = Do(func() error { calls++; return nil }, 0, time.Millisecond)
	if calls != 1 {
		t.Errorf("zero maxAttempts: fn called %d times, want 1", calls)
	}
}
