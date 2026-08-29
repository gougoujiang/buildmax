package conversation

import (
	"strings"
	"testing"

	coreconv "github.com/gougoujiang/buildmax/internal/core/conversation"
)

// A task result is stored with role "user" so the model replays it as input,
// and the system channel so the transcript knows nobody typed it. The role is
// replayed as stored: nothing rewrites it on the way out.
func TestReplayMessageFromStoreKeepsASystemChannelMessageAsInput(t *testing.T) {
	channel := "system"
	msg := replayMessageFromStore(coreconv.Message{
		Role:    "user",
		Content: "[Task Result] status: succeeded",
		Channel: &channel,
	})
	if msg.Role != "user" {
		t.Fatalf("replayMessageFromStore.Role = %q, want user", msg.Role)
	}
	if msg.Content != "[Task Result] status: succeeded" {
		t.Fatalf("replayMessageFromStore.Content = %q", msg.Content)
	}
}

func TestReplayMessageFromStore_UserRolePassthrough(t *testing.T) {
	channel := "portal"
	msg := replayMessageFromStore(coreconv.Message{
		Role:    "user",
		Content: "hello",
		Channel: &channel,
	})
	if msg.Role != "user" {
		t.Fatalf("replayMessageFromStore(user).Role = %q, want user", msg.Role)
	}
}

// A Tier 1 conversation resumes from the stored rows on every turn, so
// reasoning state has to survive the round trip through the message table or a
// second turn would send the protocol a turn it rejects.
func TestReplayMessageFromStore_CarriesReasoningState(t *testing.T) {
	stored := `{"protocol":"anthropic","data":[{"type":"thinking","signature":"sig-1"}]}`
	msg := replayMessageFromStore(coreconv.Message{
		Role:              "assistant",
		Content:           "done",
		ProviderStateJSON: &stored,
	})
	if !msg.ProviderState.Belongs("anthropic") {
		t.Fatalf("ProviderState = %+v, want the stored anthropic state", msg.ProviderState)
	}
	if !strings.Contains(string(msg.ProviderState.Data), "sig-1") {
		t.Errorf("state %s lost the signature", msg.ProviderState.Data)
	}
}

// A row written before reasoning state existed, and one holding something that
// no longer parses, both replay as a message without state rather than failing
// the turn: a conversation without reasoning continuity still works.
func TestReplayMessageFromStore_ToleratesMissingAndUnreadableState(t *testing.T) {
	unreadable := "{not json"
	for name, stored := range map[string]*string{
		"absent":     nil,
		"unreadable": &unreadable,
	} {
		t.Run(name, func(t *testing.T) {
			msg := replayMessageFromStore(coreconv.Message{
				Role: "assistant", Content: "done", ProviderStateJSON: stored,
			})
			if msg.ProviderState != nil {
				t.Errorf("ProviderState = %+v, want none", msg.ProviderState)
			}
			if msg.Content != "done" {
				t.Errorf("Content = %q, want the message to survive", msg.Content)
			}
		})
	}
}
