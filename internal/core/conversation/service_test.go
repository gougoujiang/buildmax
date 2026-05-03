package conversation

import "testing"

func TestConversationToolRunners_systemChannelDisablesTaskTools(t *testing.T) {
	svc := &ConversationService{}

	runners := svc.conversationToolRunners("c_1", "u_1", ChannelSystem)

	if runners != nil {
		t.Fatalf("system channel runners = %#v, want nil", runners)
	}
}

func TestConversationToolRunners_portalChannelAllowsTaskTools(t *testing.T) {
	svc := &ConversationService{}

	runners := svc.conversationToolRunners("c_1", "u_1", ChannelPortal)

	if runners == nil {
		t.Fatal("portal channel runners = nil, want configured container")
	}
}
