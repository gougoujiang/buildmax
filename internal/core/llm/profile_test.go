package llm

import "testing"

// An unknown profile is refused rather than defaulted. The default it would
// fall to is the one that spends money, so a typo in a caller or a newer client
// naming a profile this build does not have must be visible, not absorbed.
func TestCallProfileValidity(t *testing.T) {
	for _, p := range []CallProfile{
		ProfileAgentTurn, ProfileTitle, ProfileCompaction, ProfileEvaluation, ProfileProbe,
	} {
		if !p.Valid() {
			t.Errorf("%q should be a known profile", p)
		}
	}
	for _, p := range []CallProfile{"", "agent", "AGENT_TURN", "turn", "cache"} {
		if p.Valid() {
			t.Errorf("%q should not be a known profile", p)
		}
	}
}
