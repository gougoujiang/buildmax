package portalapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"buildmax/internal/core/model"
	"buildmax/internal/mock"
	wsconn "buildmax/internal/server/websocket"
	"buildmax/internal/util"

	"github.com/gorilla/websocket"
)

const wsTestSecret = "ws-test-secret"

func setupWSHandler() *Handler {
	teamID := "tm_personal_u1"
	return NewHandler(Config{
		JWTSecret:         wsTestSecret,
		CORSOrigin:        "*",
		TeamStore:         &mock.MockTeamStore{Teams: []model.Team{{TeamID: teamID, Name: "My Space", PersonalForUserID: util.PtrString("u1"), CreatedBy: "u1"}}, Members: []model.TeamMember{{TeamID: teamID, UserID: "u1", Role: model.TeamRoleOwner}}},
		ConversationStore: &mock.MockConversationStore{},
		ConnRegistry:      NewConnRegistry(),
	})
}

func dialWS(t *testing.T, server *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/teams/tm_personal_u1/ws?token=" + token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v (resp=%v)", err, resp)
	}
	return conn
}

func readEnvelope(t *testing.T, conn *websocket.Conn) wsconn.Envelope {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	env, err := wsconn.Decode(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return env
}

func sendEnvelope(t *testing.T, conn *websocket.Conn, typ string, payload any) {
	t.Helper()
	data, err := wsconn.Encode(typ, payload)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestWSUpgradeRequiresToken(t *testing.T) {
	h := setupWSHandler()
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/teams/tm_personal_u1/ws"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestWSUpgradeInvalidToken(t *testing.T) {
	h := setupWSHandler()
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/teams/tm_personal_u1/ws?token=bad"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestWSConversationCreateFlow(t *testing.T) {
	h := setupWSHandler()
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := util.SignJWT("u1", wsTestSecret)
	conn := dialWS(t, server, token)
	defer conn.Close()

	sendEnvelope(t, conn, wsconn.TypeConversationCreate, wsconn.ConversationCreate{
		Message: "hello",
	})

	env := readEnvelope(t, conn)
	if env.Type != wsconn.TypeConversationCreated {
		t.Fatalf("first event type = %q, want %q", env.Type, wsconn.TypeConversationCreated)
	}
	var created wsconn.ConversationCreated
	json.Unmarshal(env.Payload, &created)
	if created.ConversationID == "" {
		t.Error("conversation_id is empty")
	}

	// Since LLMClient is nil, we should get an error then completed
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		env = readEnvelope(t, conn)
		seen[env.Type] = true
		if env.Type == wsconn.TypeMessageCompleted {
			break
		}
	}
	if !seen[wsconn.TypeMessageCompleted] {
		t.Error("did not receive conversation.message.completed")
	}
}

func TestWSUnknownEventType(t *testing.T) {
	h := setupWSHandler()
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := util.SignJWT("u1", wsTestSecret)
	conn := dialWS(t, server, token)
	defer conn.Close()

	sendEnvelope(t, conn, "unknown.event", map[string]string{})

	env := readEnvelope(t, conn)
	if env.Type != wsconn.TypeSystemError {
		t.Errorf("type = %q, want %q", env.Type, wsconn.TypeSystemError)
	}
}
