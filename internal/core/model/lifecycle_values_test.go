package model

import "testing"

// TestActiveRunStatusesReturnsACopy keeps the shared list out of a caller's
// reach. A caller that sorts or filters it in place would change what every
// later caller sees, and the failure would surface far from the edit.
func TestActiveRunStatusesReturnsACopy(t *testing.T) {
	active := ActiveRunStatuses()
	active[0] = "BROKEN"
	if got := ActiveRunStatuses()[0]; got != string(RunStatusPending) {
		t.Fatalf("active status mutated through caller slice: %q", got)
	}
}
