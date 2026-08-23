package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/server/turnqueue"
	wsconn "github.com/gougoujiang/buildmax/internal/server/websocket"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"

	gws "github.com/gorilla/websocket"
)

// waitForMessages polls until the store holds want messages, or fails. The turn
// runs on the queue's goroutine while the test reads from its own.
func waitForMessages(t *testing.T, m *mock.MockConversationMessageStore, want int) []model.ConversationMessage {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		msgs, _ := m.ListMessages(context.Background(), "conv-1")
		if len(msgs) >= want {
			return msgs
		}
		if time.Now().After(deadline) {
			t.Fatalf("stored %d messages, want %d", len(msgs), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type replyLLMClient struct{ reply string }

func (c *replyLLMClient) ChatCompletionBlocking(ctx context.Context, req llm.Request) (llm.Completion, error) {
	return llm.Completion{Content: c.reply}, nil
}

func (c *replyLLMClient) ChatCompletionStreaming(ctx context.Context, req llm.Request, onDelta func(string)) (llm.Completion, error) {
	onDelta(c.reply)
	return llm.Completion{Content: c.reply}, nil
}

func (c *replyLLMClient) ContextWindow() int { return 0 }

func terminalInfo() model.TaskRunTerminalInfo {
	return model.TaskRunTerminalInfo{
		TaskRunID:      "tr_1",
		TaskID:         "tk_1",
		ConversationID: "conv-1",
		TeamID:         "tm_shared",
		UserID:         "u1",
		Status:         string(model.RunStatusSucceeded),
		Output:         util.Ptr("the analysis found three problems"),
	}
}

// A run finishes whether or not the person who started it is watching, so the
// turn that reports it must not need a connection to run. It used to be skipped
// outright when the creator had no socket open.
func TestTaskResultTurnRunsWithNobodyConnected(t *testing.T) {
	messages := &mock.MockConversationMessageStore{}
	h := NewHandler(Config{
		JWTSecret:                wsTestSecret,
		ConversationStore:        &mock.MockConversationStore{},
		ConversationMessageStore: messages,
		ConversationLLMClient:    &replyLLMClient{reply: "Three problems turned up."},
	})

	h.reportTaskRunTerminal(context.Background(), terminalInfo())

	stored := waitForMessages(t, messages, 2)
	if stored[0].Channel == nil || *stored[0].Channel != conversation.ChannelSystem {
		t.Fatalf("incoming message channel = %v, want %q", stored[0].Channel, conversation.ChannelSystem)
	}
	if stored[1].Role != "assistant" || stored[1].Content != "Three problems turned up." {
		t.Errorf("reply = %q/%q, want the presenter's summary", stored[1].Role, stored[1].Content)
	}
}

// The result stays reachable when the presenter turn fails: the card's own
// invalidation went out first and does not depend on the turn.
func TestTaskRunTerminalBroadcastSurvivesPresenterFailure(t *testing.T) {
	teamID := "tm_shared"
	h := NewHandler(Config{
		JWTSecret:  wsTestSecret,
		CORSOrigin: "*",
		TeamStore:  sharedTeamStore(teamID),
		// No conversation LLM: the presenter turn cannot run at all.
		ConversationStore: &mock.MockConversationStore{},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	conn := dialSharedTeamWS(t, server, "u1")
	defer conn.Close()

	h.reportTaskRunTerminal(context.Background(), terminalInfo())

	env := readEnvelope(t, conn)
	if env.Type != wsconn.TypeTaskStatusChanged {
		t.Fatalf("event = %q, want %q", env.Type, wsconn.TypeTaskStatusChanged)
	}
	var changed wsconn.TaskStatusChanged
	if err := json.Unmarshal(env.Payload, &changed); err != nil {
		t.Fatal(err)
	}
	if changed.TaskID != "tk_1" || changed.Status != string(model.RunStatusSucceeded) {
		t.Errorf("payload = %+v, want the finished task", changed)
	}
}

// Every connection on the team is told, not the creator's first one. Two tabs and
// a teammate all show the same conversation.
func TestTaskRunTerminalBroadcastsToEveryTeamConnection(t *testing.T) {
	teamID := "tm_shared"
	h := NewHandler(Config{
		JWTSecret:         wsTestSecret,
		CORSOrigin:        "*",
		TeamStore:         sharedTeamStore(teamID),
		ConversationStore: &mock.MockConversationStore{},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	creatorTab := dialSharedTeamWS(t, server, "u1")
	defer creatorTab.Close()
	secondTab := dialSharedTeamWS(t, server, "u1")
	defer secondTab.Close()
	teammate := dialSharedTeamWS(t, server, "u2")
	defer teammate.Close()

	h.reportTaskRunTerminal(context.Background(), terminalInfo())

	for name, conn := range map[string]*gws.Conn{
		"creator's first tab":  creatorTab,
		"creator's second tab": secondTab,
		"teammate":             teammate,
	} {
		if env := readEnvelope(t, conn); env.Type != wsconn.TypeTaskStatusChanged {
			t.Errorf("%s got %q, want %q", name, env.Type, wsconn.TypeTaskStatusChanged)
		}
	}
}

// A conversation busy with the user's own turn does not lose the report: it
// queues behind that turn like any other.
func TestTaskResultTurnQueuesBehindARunningTurn(t *testing.T) {
	messages := &mock.MockConversationMessageStore{}
	h := NewHandler(Config{
		JWTSecret:                wsTestSecret,
		ConversationStore:        &mock.MockConversationStore{},
		ConversationMessageStore: messages,
		ConversationLLMClient:    &replyLLMClient{reply: "done"},
	})

	started := make(chan struct{})
	release := make(chan struct{})
	held := turnqueue.NewJob(func() {
		close(started)
		<-release
	})
	if _, err := h.turns.Submit("conv-1", held); err != nil {
		t.Fatal(err)
	}
	<-started

	h.reportTaskRunTerminal(context.Background(), terminalInfo())
	close(release)

	waitForMessages(t, messages, 2)
}

func sharedTeamStore(teamID string) *mock.MockTeamStore {
	return &mock.MockTeamStore{
		Teams: []model.Team{{ID: teamID, Name: "Shared", CreatedBy: "u1"}},
		Members: []model.TeamMember{
			{TeamID: teamID, UserID: "u1", Role: model.TeamRoleOwner},
			{TeamID: teamID, UserID: "u2", Role: model.TeamRoleMember},
		},
	}
}

func dialSharedTeamWS(t *testing.T, server *httptest.Server, userID string) *gws.Conn {
	t.Helper()
	token := testsupport.SignJWT(userID, wsTestSecret)
	wsURL := "ws" + server.URL[len("http"):] + "/api/teams/tm_shared/ws?token=" + token
	conn, resp, err := gws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v (resp=%v)", err, resp)
	}
	// The upgrade completes before the server registers the connection. Round-trip
	// one event so a broadcast sent right after this returns cannot miss it.
	sendEnvelope(t, conn, "unknown.event", map[string]string{})
	if env := readEnvelope(t, conn); env.Type != wsconn.TypeSystemError {
		t.Fatalf("handshake event = %q, want %q", env.Type, wsconn.TypeSystemError)
	}
	return conn
}
