package conversation

import (
	"testing"

	convchannel "github.com/gougoujiang/buildmax/internal/service/conversation/channel"
)

func TestConversationTurn_fields(t *testing.T) {
	raw := map[string]any{"key": "val"}
	turn := convchannel.Turn{
		Channel:        ChannelPortal,
		ConversationID: "c_xyz",
		UserID:         "u_123",
		Message:        "hello",
		Raw:            raw,
	}
	if turn.Channel != ChannelPortal || turn.ConversationID != "c_xyz" {
		t.Errorf("Channel/ConversationID: got %q %q", turn.Channel, turn.ConversationID)
	}
	if turn.UserID != "u_123" || turn.Message != "hello" {
		t.Errorf("UserID/Message: got %q %q", turn.UserID, turn.Message)
	}
	if turn.Raw["key"] != "val" {
		t.Errorf("Raw: got %v", turn.Raw)
	}
}

func TestConversationResult_fields(t *testing.T) {
	result := ConversationResult{
		Reply: "Sure, I'll do that.",
		Runs:  []SpawnedRun{{TaskID: "t_1", RunID: "r_001"}, {TaskID: "t_2", RunID: "r_002"}},
	}
	if result.Reply != "Sure, I'll do that." {
		t.Errorf("Reply: got %q", result.Reply)
	}
	if len(result.Runs) != 2 || result.Runs[0].RunID != "r_001" || result.Runs[1].TaskID != "t_2" {
		t.Errorf("Runs: got %v", result.Runs)
	}
}

func TestChannelConstants_nonEmptyAndDistinct(t *testing.T) {
	seen := make(map[string]bool)
	for _, ch := range ValidChannels {
		if ch == "" {
			t.Errorf("channel constant is empty")
		}
		if seen[ch] {
			t.Errorf("duplicate channel %q", ch)
		}
		seen[ch] = true
	}
	if len(ValidChannels) != 4 {
		t.Errorf("expected 4 channel constants, got %d", len(ValidChannels))
	}
	// Ensure the named constants match
	if ChannelPortal != "portal" || ChannelTelegram != "telegram" || ChannelCron != "cron" || ChannelWebhook != "webhook" {
		t.Errorf("channel constant values changed")
	}
}

func TestValidChannel(t *testing.T) {
	for _, ch := range ValidChannels {
		if !ValidChannel(ch) {
			t.Errorf("ValidChannel(%q) = false, want true", ch)
		}
	}
	if ValidChannel("") {
		t.Error("ValidChannel(\"\") = true, want false")
	}
	if ValidChannel("slack") {
		t.Error("ValidChannel(\"slack\") = true, want false")
	}
}
