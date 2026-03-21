package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"buildmax/internal/streamhub"
	"buildmax/internal/testutil"
	"buildmax/internal/wsconn"

	"github.com/gorilla/websocket"
)

const wsTestSecret = "ws-test-secret"

func setupWSHandler(hub streamhub.StreamHub) *Handler {
	return NewHandler(Config{
		JWTSecret:         wsTestSecret,
		CORSOrigin:        "*",
		ConversationStore: &testutil.MockConversationStore{},
		Hub:               hub,
	})
}

func dialWS(t *testing.T, server *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws?token=" + token
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
	h := setupWSHandler(nil)
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestWSUpgradeInvalidToken(t *testing.T) {
	h := setupWSHandler(nil)
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws?token=bad"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestWSConversationCreateFlow(t *testing.T) {
	h := setupWSHandler(nil)
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := testutil.SignJWT("u1", wsTestSecret)
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

	// Since LLMCaller is nil, we should get an error then completed
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

func TestWSTaskStreamSubscription(t *testing.T) {
	hub := streamhub.NewStreamHub()
	h := setupWSHandler(hub)
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := testutil.SignJWT("u1", wsTestSecret)
	conn := dialWS(t, server, token)
	defer conn.Close()

	taskID := "c_test123"

	sendEnvelope(t, conn, wsconn.TypeSubscribeTask, wsconn.SubscribeTask{TaskID: taskID})

	// Give subscription time to set up
	time.Sleep(50 * time.Millisecond)

	hub.Append(taskID, "chunk1")
	hub.Append(taskID, "chunk2")
	hub.Done(taskID)

	var deltas []string
	gotDone := false
	for i := 0; i < 5; i++ {
		env := readEnvelope(t, conn)
		switch env.Type {
		case wsconn.TypeTaskStreamDelta:
			var d wsconn.TaskStreamDelta
			json.Unmarshal(env.Payload, &d)
			deltas = append(deltas, d.Delta)
		case wsconn.TypeTaskStreamDone:
			gotDone = true
		}
		if gotDone {
			break
		}
	}

	if len(deltas) == 0 {
		t.Error("received no task stream deltas")
	}
	if !gotDone {
		t.Error("did not receive task.stream.done")
	}
}

func TestWSUnknownEventType(t *testing.T) {
	h := setupWSHandler(nil)
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := testutil.SignJWT("u1", wsTestSecret)
	conn := dialWS(t, server, token)
	defer conn.Close()

	sendEnvelope(t, conn, "unknown.event", map[string]string{})

	env := readEnvelope(t, conn)
	if env.Type != wsconn.TypeSystemError {
		t.Errorf("type = %q, want %q", env.Type, wsconn.TypeSystemError)
	}
}
