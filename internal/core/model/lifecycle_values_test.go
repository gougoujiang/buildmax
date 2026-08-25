package model

import "testing"

func TestLifecycleListsReturnCopies(t *testing.T) {
	active := ActiveRunStatuses()
	active[0] = "BROKEN"
	if got := ActiveRunStatuses()[0]; got != string(RunStatusPending) {
		t.Fatalf("active status mutated through caller slice: %q", got)
	}

	synthetic := SyntheticChannels()
	synthetic[0] = "BROKEN"
	if got := SyntheticChannels()[0]; got != ChannelWorkflow {
		t.Fatalf("synthetic channel mutated through caller slice: %q", got)
	}
}
