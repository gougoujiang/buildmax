// Package schedule holds the cron and application logic for recurring Agent
// schedules. The domain model and its store contract live in
// internal/core/schedule; this package is where the cron library and timezone
// handling that core deliberately excludes belong.
package schedule

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// standardParser accepts the five standard cron fields (minute, hour, day of
// month, month, day of week) and the @-descriptors. Seconds are intentionally
// not a field: the smallest schedule granularity is one minute.
var standardParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// Validate reports whether a cron expression and IANA timezone are both usable.
// It is the check the write path runs so an invalid schedule is refused at
// creation rather than failing silently when the dispatcher first reaches it.
func Validate(cronExpr, timezone string) error {
	if _, err := time.LoadLocation(timezone); err != nil {
		return fmt.Errorf("timezone %q: %w", timezone, err)
	}
	if _, err := standardParser.Parse(cronExpr); err != nil {
		return fmt.Errorf("cron %q: %w", cronExpr, err)
	}
	return nil
}

// Next returns the first firing strictly after `after`, evaluated in the
// schedule's timezone so a wall-clock time survives DST, and returned in UTC to
// match how instants are stored and compared.
//
// Because the next time is computed from `after` rather than from a stale stored
// due time, a schedule the server missed while it was down fires once and
// resumes: the missed slots between the old due time and now are skipped rather
// than replayed.
func Next(cronExpr, timezone string, after time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("timezone %q: %w", timezone, err)
	}
	sched, err := standardParser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron %q: %w", cronExpr, err)
	}
	return sched.Next(after.In(loc)).UTC(), nil
}
