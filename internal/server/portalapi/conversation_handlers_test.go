package portalapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"buildmax/internal/infra/db"
	"buildmax/internal/testutil"
)

type mockConversationMessageStore struct {
	messages []db.ConversationMessage
}

func (m *mockConversationMessageStore) AppendMessage(ctx context.Context, conversationID, role, content string, channel *string, toolCallID *string, toolCallsJSON *string) (*db.ConversationMessage, error) {
	msg := db.ConversationMessage{
		ConversationMessageID: "cm_mock",
		ConversationID:        conversationID,
		Role:                  role,
		Content:               content,
		Channel:               channel,
		ToolCallID:            toolCallID,
		ToolCallsJSON:         toolCallsJSON,
	}
	m.messages = append(m.messages, msg)
	return &msg, nil
}

func (m *mockConversationMessageStore) ListMessages(ctx context.Context, conversationID string) ([]db.ConversationMessage, error) {
	var out []db.ConversationMessage
	for _, msg := range m.messages {
		if msg.ConversationID == conversationID {
			out = append(out, msg)
		}
	}
	return out, nil
}

func TestGetConversationMessagesHandler_HidesSystemMessages(t *testing.T) {
	secret := "test-conversation-secret"
	conversationID := "conv1"
	teamID := "tm_personal_u1"
	channel := "system"
	messageStore := &mockConversationMessageStore{
		messages: []db.ConversationMessage{
			{ConversationMessageID: "cm_1", ConversationID: conversationID, Role: "user", Content: "hello", CreatedAt: 1},
			{ConversationMessageID: "cm_tool", ConversationID: conversationID, Role: "tool", Content: "tool output", CreatedAt: 2},
			{ConversationMessageID: "cm_2", ConversationID: conversationID, Role: "system", Content: "[Task Result] internal", Channel: &channel, CreatedAt: 2},
			{ConversationMessageID: "cm_3", ConversationID: conversationID, Role: "assistant", Content: "final reply", CreatedAt: 3},
		},
	}
	h := NewHandler(Config{
		JWTSecret: secret,
		TeamStore: &testutil.MockTeamStore{
			Teams:   []db.Team{{TeamID: teamID, Name: "My Space", PersonalForUserID: testutil.PtrString("u1"), CreatedBy: "u1"}},
			Members: []db.TeamMember{{TeamID: teamID, UserID: "u1", Role: db.TeamRoleOwner}},
		},
		ConversationStore: &testutil.MockConversationStore{
			Conversations: []db.Conversation{
				{ConversationID: conversationID, UserID: "u1", TeamID: teamID, Channel: "portal", CreatedBy: "u1", CreatedAt: 123},
			},
		},
		ConversationMessageStore: messageStore,
	})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/conversations/"+conversationID+"/messages", nil)
	req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", secret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var out messagesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(out.Messages))
	}
	if out.Messages[0].Role != "user" || out.Messages[1].Role != "assistant" {
		t.Fatalf("roles = [%q, %q], want [user, assistant]", out.Messages[0].Role, out.Messages[1].Role)
	}
}
