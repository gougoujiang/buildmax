package work

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coreconv "github.com/gougoujiang/buildmax/internal/core/conversation"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

func TestGetConversationMessagesHandler_HidesSystemMessages(t *testing.T) {
	secret := "test-conversation-secret"
	conversationID := "conv1"
	teamID := "tm_personal_u1"
	channel := "system"
	messageStore := &mock.MockConversationMessageStore{
		Messages: []coreconv.Message{
			{ID: "cm_1", ConversationID: conversationID, Role: "user", Content: "hello", CreatedAt: time.Unix(1, 0).UTC()},
			{ID: "cm_tool", ConversationID: conversationID, Role: "tool", Content: "tool output", CreatedAt: time.Unix(2, 0).UTC()},
			{ID: "cm_2", ConversationID: conversationID, Role: "system", Content: "[Task Result] internal", Channel: &channel, CreatedAt: time.Unix(2, 0).UTC()},
			// What the runtime writes today: role "user" so the model replays it,
			// system channel so the transcript knows the user did not type it.
			{ID: "cm_task_result", ConversationID: conversationID, Role: "user", Content: "[Task Result] task_id: tk_1 | status: succeeded", Channel: &channel, CreatedAt: time.Unix(2, 0).UTC()},
			{ID: "cm_3", ConversationID: conversationID, Role: "assistant", Content: "final reply", CreatedAt: time.Unix(3, 0).UTC()},
		},
	}
	h := New(Config{
		JWTSecret: secret,
		Teams: &mock.MockTeamStore{
			Teams:   []coreteam.Team{{ID: teamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}},
			Members: []coreteam.Member{{TeamID: teamID, UserID: "u1", Role: coreteam.RoleOwner}},
		},
		Conversations: &mock.MockConversationStore{
			Conversations: []coreconv.Conversation{
				{ID: conversationID, UserID: "u1", TeamID: teamID, Channel: "portal", CreatedBy: "u1", CreatedAt: time.Unix(123, 0).UTC()},
			},
		},
		Messages: messageStore,
	})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/conversations/"+conversationID+"/messages", nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", secret))
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

func TestListConversationsReturnsTeamConversations(t *testing.T) {
	secret := "test-conversation-list-secret"
	teamID := "tm_personal_u1"
	h := New(Config{
		JWTSecret: secret,
		Teams: &mock.MockTeamStore{
			Teams:   []coreteam.Team{{ID: teamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}},
			Members: []coreteam.Member{{TeamID: teamID, UserID: "u1", Role: coreteam.RoleOwner}},
		},
		Conversations: &mock.MockConversationStore{Conversations: []coreconv.Conversation{
			{ID: "conv_mine", UserID: "u1", TeamID: teamID, Channel: "portal", CreatedBy: "u1"},
			{ID: "conv_hook", UserID: "u1", TeamID: teamID, Channel: "webhook", CreatedBy: "u1"},
		}},
	})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/conversations", nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", secret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var out conversationListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out.Total != 2 || len(out.Conversations) != 2 {
		t.Fatalf("conversations = %+v, total = %d, want both team conversations", out.Conversations, out.Total)
	}
}

// A conversation on a synthetic channel is one the server made and nobody
// holds: the Portal renders it as agent-owned and the list hides it. A caller
// that could name one would be creating a conversation it then cannot see.
func TestCreateConversationRejectsAChannelTheCallerMayNotClaim(t *testing.T) {
	secret := "test-conversation-secret"
	teamID := "tm_personal_u1"
	h := New(Config{
		JWTSecret: secret,
		Teams: &mock.MockTeamStore{
			Teams:   []coreteam.Team{{ID: teamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}},
			Members: []coreteam.Member{{TeamID: teamID, UserID: "u1", Role: coreteam.RoleOwner}},
		},
		Conversations: &mock.MockConversationStore{},
		Messages:      &mock.MockConversationMessageStore{},
	})
	mux := http.NewServeMux()
	h.Register(mux)

	for _, channel := range []string{"issue_agent", "workflow", "system", "slack"} {
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+teamID+"/conversations",
			strings.NewReader(`{"channel":"`+channel+`"}`))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", secret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("channel %q: status = %d, want 400; body = %s", channel, rec.Code, rec.Body.String())
		}
	}
}

// An absent channel is the Portal, and the transport channels stay accepted.
func TestCreateConversationAcceptsTheTransportChannels(t *testing.T) {
	secret := "test-conversation-secret"
	teamID := "tm_personal_u1"
	for _, body := range []string{`{}`, `{"channel":"portal"}`, `{"channel":"telegram"}`, `{"channel":"cron"}`, `{"channel":"webhook"}`} {
		h := New(Config{
			JWTSecret: secret,
			Teams: &mock.MockTeamStore{
				Teams:   []coreteam.Team{{ID: teamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}},
				Members: []coreteam.Member{{TeamID: teamID, UserID: "u1", Role: coreteam.RoleOwner}},
			},
			Conversations: &mock.MockConversationStore{},
			Messages:      &mock.MockConversationMessageStore{},
		})
		mux := http.NewServeMux()
		h.Register(mux)

		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+teamID+"/conversations", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", secret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Errorf("body %s: status = %d, want 201; body = %s", body, rec.Code, rec.Body.String())
		}
	}
}
