package schedule

import (
	"testing"
	"time"
)

func TestNextEvaluatesInTimezoneAndReturnsUTC(t *testing.T) {
	// 09:00 daily in Shanghai (UTC+8) is 01:00 UTC.
	after := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := Next("0 9 * * *", "Asia/Shanghai", after)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := time.Date(2000, 1, 1, 1, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("Next = %v, want %v (09:00 Shanghai in UTC)", next, want)
	}
	if next.Location() != time.UTC {
		t.Errorf("Next location = %v, want UTC", next.Location())
	}
}

func TestNextIsStrictlyAfter(t *testing.T) {
	at := time.Date(2000, 1, 1, 9, 0, 0, 0, time.UTC)
	next, err := Next("0 * * * *", "UTC", at)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !next.After(at) {
		t.Errorf("Next(%v) = %v, want a time strictly after", at, next)
	}
}

func TestValidate(t *testing.T) {
	if err := Validate("0 9 * * *", "Asia/Shanghai"); err != nil {
		t.Errorf("a valid schedule was rejected: %v", err)
	}
	if err := Validate("nonsense", "UTC"); err == nil {
		t.Error("an invalid cron expression was accepted")
	}
	if err := Validate("0 9 * * *", "Mars/Phobos"); err == nil {
		t.Error("an unknown timezone was accepted")
	}
}
