package portal

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	convapp "buildmax/internal/app/conversation"
	"buildmax/internal/streamhub"
	"buildmax/internal/wsconn"

	"github.com/gorilla/websocket"
)

const (
	wsWriteChSize  = 256
	wsPingInterval = 30 * time.Second
	wsReadDeadline = 60 * time.Second
	wsWriteWait    = 10 * time.Second
)

// wsConn manages a single WebSocket connection for one authenticated user.
type wsConn struct {
	conn   *websocket.Conn
	h      *Handler
	userID string

	writeCh chan []byte
	closed  chan struct{} // closed when connection is being torn down

	taskSubsMu sync.Mutex
	taskSubs   map[string]func() // taskID → unsub

	turnMu sync.Mutex // serializes conversation turns

	cancel context.CancelFunc
}

// wsSink implements llm.StreamSink by sending conversation.message.delta events.
type wsSink struct {
	c              *wsConn
	conversationID string
}

func (s *wsSink) OnDelta(delta string) {
	s.c.sendEvent(wsconn.TypeMessageDelta, wsconn.MessageDelta{
		ConversationID: s.conversationID,
		Delta:          delta,
	})
}

func (h *Handler) wsUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "token required", http.StatusUnauthorized)
		return
	}
	userID, ok := userIDFromToken(tokenStr, h.cfg.JWTSecret)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if h.cfg.CORSOrigin == "" || h.cfg.CORSOrigin == "*" {
				return true
			}
			origin := r.Header.Get("Origin")
			return origin == "" || origin == h.cfg.CORSOrigin
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade failed", "err", err, "user_id", userID)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	wc := &wsConn{
		conn:     conn,
		h:        h,
		userID:   userID,
		writeCh:  make(chan []byte, wsWriteChSize),
		closed:   make(chan struct{}),
		taskSubs: make(map[string]func()),
		cancel:   cancel,
	}

	slog.Info("ws connected", "user_id", userID, "remote", r.RemoteAddr)
	go wc.writeLoop(ctx)
	wc.readLoop(ctx) // blocks until the connection closes
}

func (wc *wsConn) readLoop(ctx context.Context) {
	defer func() {
		slog.Info("ws disconnected", "user_id", wc.userID)
		wc.cleanup()
		wc.conn.Close()
	}()

	wc.conn.SetReadDeadline(time.Now().Add(wsReadDeadline))
	wc.conn.SetPongHandler(func(string) error {
		wc.conn.SetReadDeadline(time.Now().Add(wsReadDeadline))
		return nil
	})

	for {
		_, data, err := wc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Info("ws read error", "err", err, "user_id", wc.userID)
			}
			return
		}
		wc.conn.SetReadDeadline(time.Now().Add(wsReadDeadline))

		env, err := wsconn.Decode(data)
		if err != nil {
			slog.Warn("ws recv invalid message", "user_id", wc.userID, "err", err)
			wc.sendEvent(wsconn.TypeSystemError, wsconn.SystemError{Error: "invalid message format"})
			continue
		}
		slog.Debug("ws recv", "user_id", wc.userID, "type", env.Type)
		wc.handleClientEvent(ctx, env)
	}
}

func (wc *wsConn) writeLoop(ctx context.Context) {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			wc.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		case msg, ok := <-wc.writeCh:
			if !ok {
				wc.conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			wc.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := wc.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				slog.Info("ws write error", "err", err, "user_id", wc.userID)
				return
			}
		case <-ticker.C:
			wc.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := wc.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (wc *wsConn) sendEvent(eventType string, payload any) {
	data, err := wsconn.Encode(eventType, payload)
	if err != nil {
		slog.Warn("ws encode error", "type", eventType, "err", err)
		return
	}
	slog.Debug("ws send", "user_id", wc.userID, "type", eventType)
	select {
	case <-wc.closed:
		return
	default:
	}
	select {
	case wc.writeCh <- data:
	case <-wc.closed:
	default:
		slog.Warn("ws write channel full, dropping event", "type", eventType, "user_id", wc.userID)
	}
}

func (wc *wsConn) handleClientEvent(ctx context.Context, env wsconn.Envelope) {
	switch env.Type {
	case wsconn.TypeConversationCreate:
		p, err := wsconn.DecodePayload[wsconn.ConversationCreate](env)
		if err != nil {
			wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{Error: "invalid payload"})
			return
		}
		wc.handleConversationCreate(ctx, p)

	case wsconn.TypeConversationMessage:
		p, err := wsconn.DecodePayload[wsconn.ConversationMessage](env)
		if err != nil {
			wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{Error: "invalid payload"})
			return
		}
		wc.handleConversationMessage(ctx, p)

	case wsconn.TypeSubscribeTask:
		p, err := wsconn.DecodePayload[wsconn.SubscribeTask](env)
		if err != nil {
			wc.sendEvent(wsconn.TypeSystemError, wsconn.SystemError{Error: "invalid payload"})
			return
		}
		wc.handleSubscribeTask(ctx, p)

	case wsconn.TypeUnsubscribeTask:
		p, err := wsconn.DecodePayload[wsconn.UnsubscribeTask](env)
		if err != nil {
			return
		}
		wc.handleUnsubscribeTask(p)

	default:
		wc.sendEvent(wsconn.TypeSystemError, wsconn.SystemError{Error: "unknown event type: " + env.Type})
	}
}

func (wc *wsConn) handleConversationCreate(ctx context.Context, p wsconn.ConversationCreate) {
	if p.Message == "" {
		wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{Error: "message required"})
		return
	}
	if wc.h.cfg.ConversationStore == nil {
		wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{Error: "conversations not configured"})
		return
	}

	channel := p.Channel
	if channel == "" {
		channel = "portal"
	}

	conv, err := wc.h.cfg.ConversationStore.CreateConversation(ctx, wc.userID, channel, wc.userID)
	if err != nil {
		slog.Error("ws create conversation", "err", err, "user_id", wc.userID)
		wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{Error: "failed to create conversation"})
		return
	}

	slog.Info("ws conversation created", "user_id", wc.userID, "conversation_id", conv.ConversationID)
	wc.sendEvent(wsconn.TypeConversationCreated, wsconn.ConversationCreated{ConversationID: conv.ConversationID})
	wc.runConversationTurn(ctx, conv.ConversationID, p.Message, channel)
}

func (wc *wsConn) handleConversationMessage(ctx context.Context, p wsconn.ConversationMessage) {
	if p.ConversationID == "" || p.Content == "" {
		wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{
			ConversationID: p.ConversationID,
			Error:          "conversation_id and content required",
		})
		return
	}
	if wc.h.cfg.ConversationStore == nil {
		wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{
			ConversationID: p.ConversationID,
			Error:          "conversations not configured",
		})
		return
	}

	conv, err := wc.h.cfg.ConversationStore.GetConversation(ctx, p.ConversationID)
	if err != nil {
		slog.Error("ws get conversation", "err", err, "conversation_id", p.ConversationID)
		wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{
			ConversationID: p.ConversationID,
			Error:          "failed to load conversation",
		})
		return
	}
	if conv == nil || conv.UserID != wc.userID {
		wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{
			ConversationID: p.ConversationID,
			Error:          "conversation not found",
		})
		return
	}

	slog.Info("ws conversation message", "user_id", wc.userID, "conversation_id", p.ConversationID)
	wc.runConversationTurn(ctx, p.ConversationID, p.Content, conv.Channel)
}

func (wc *wsConn) runConversationTurn(ctx context.Context, conversationID, message, channel string) {
	if !wc.turnMu.TryLock() {
		wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{
			ConversationID: conversationID,
			Error:          "a conversation turn is already in progress",
		})
		return
	}

	go func() {
		defer wc.turnMu.Unlock()

		slog.Info("ws turn start", "user_id", wc.userID, "conversation_id", conversationID)
		sink := &wsSink{c: wc, conversationID: conversationID}
		svc := wc.h.conversationService()

		_, err := svc.HandleTurn(ctx, convapp.HandleTurnCmd{
			UserID:         wc.userID,
			Channel:        channel,
			Message:        message,
			ConversationID: conversationID,
			StreamSink:     sink,
		})
		if err != nil {
			slog.Error("ws turn error", "user_id", wc.userID, "conversation_id", conversationID, "err", err)
			wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{
				ConversationID: conversationID,
				Error:          err.Error(),
			})
		}
		slog.Info("ws turn done", "user_id", wc.userID, "conversation_id", conversationID)
		wc.sendEvent(wsconn.TypeMessageCompleted, wsconn.MessageCompleted{
			ConversationID: conversationID,
		})
	}()
}

func (wc *wsConn) handleSubscribeTask(ctx context.Context, p wsconn.SubscribeTask) {
	if p.TaskID == "" {
		wc.sendEvent(wsconn.TypeSystemError, wsconn.SystemError{Error: "task_id required"})
		return
	}
	if wc.h.cfg.Hub == nil {
		wc.sendEvent(wsconn.TypeSystemError, wsconn.SystemError{Error: "stream not available"})
		return
	}
	if wc.h.cfg.TaskStore != nil {
		task, err := wc.h.cfg.TaskStore.GetTask(ctx, p.TaskID)
		if err != nil || task == nil {
			wc.sendEvent(wsconn.TypeSystemError, wsconn.SystemError{Error: "task not found"})
			return
		}
		if wc.h.cfg.ConversationStore != nil {
			conv, err := wc.h.cfg.ConversationStore.GetConversation(ctx, task.ConversationID)
			if err != nil || conv == nil || conv.UserID != wc.userID {
				wc.sendEvent(wsconn.TypeSystemError, wsconn.SystemError{Error: "task not found"})
				return
			}
		}
	}

	wc.taskSubsMu.Lock()
	if existingUnsub, ok := wc.taskSubs[p.TaskID]; ok {
		existingUnsub()
		delete(wc.taskSubs, p.TaskID)
	}
	wc.taskSubsMu.Unlock()

	slog.Info("ws task subscribe", "user_id", wc.userID, "task_id", p.TaskID)
	events, unsub := wc.h.cfg.Hub.Subscribe(p.TaskID)

	wc.taskSubsMu.Lock()
	wc.taskSubs[p.TaskID] = unsub
	wc.taskSubsMu.Unlock()

	if buf := wc.h.cfg.Hub.Buffer(p.TaskID); buf != "" {
		wc.sendEvent(wsconn.TypeTaskStreamDelta, wsconn.TaskStreamDelta{
			TaskID: p.TaskID,
			Delta:  buf,
		})
	}

	go func() {
		defer func() {
			wc.taskSubsMu.Lock()
			delete(wc.taskSubs, p.TaskID)
			wc.taskSubsMu.Unlock()
		}()
		for {
			select {
			case <-ctx.Done():
				unsub()
				return
			case msg, ok := <-events:
				if !ok {
					return
				}
				if msg == streamhub.StreamEventDone {
					wc.sendEvent(wsconn.TypeTaskStreamDone, wsconn.TaskStreamDone{TaskID: p.TaskID})
					return
				}
				wc.sendEvent(wsconn.TypeTaskStreamDelta, wsconn.TaskStreamDelta{
					TaskID: p.TaskID,
					Delta:  msg,
				})
			}
		}
	}()
}

func (wc *wsConn) handleUnsubscribeTask(p wsconn.UnsubscribeTask) {
	slog.Info("ws task unsubscribe", "user_id", wc.userID, "task_id", p.TaskID)
	wc.taskSubsMu.Lock()
	defer wc.taskSubsMu.Unlock()
	if unsub, ok := wc.taskSubs[p.TaskID]; ok {
		unsub()
		delete(wc.taskSubs, p.TaskID)
	}
}

func (wc *wsConn) cleanup() {
	wc.cancel()

	// Signal all senders to stop before closing the channel.
	select {
	case <-wc.closed:
	default:
		close(wc.closed)
	}

	wc.taskSubsMu.Lock()
	for id, unsub := range wc.taskSubs {
		unsub()
		delete(wc.taskSubs, id)
	}
	wc.taskSubsMu.Unlock()

	close(wc.writeCh)
}

// userIDFromToken validates a JWT token string and returns the user ID (sub claim).
func userIDFromToken(tokenStr string, jwtSecret string) (string, bool) {
	if jwtSecret == "" || tokenStr == "" {
		return "", false
	}
	tokenStr = strings.TrimSpace(tokenStr)
	return parseJWTSub(tokenStr, jwtSecret)
}
