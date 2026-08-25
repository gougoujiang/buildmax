package conversation

import "testing"

// TestSyntheticChannelsReturnsACopy keeps the shared list out of a caller's
// reach. A caller that sorts or filters it in place would change what every
// later caller sees, and the failure would surface far from the edit.
func TestSyntheticChannelsReturnsACopy(t *testing.T) {
	synthetic := SyntheticChannels()
	synthetic[0] = "BROKEN"
	if got := SyntheticChannels()[0]; got != ChannelWorkflow {
		t.Fatalf("synthetic channel mutated through caller slice: %q", got)
	}
}
