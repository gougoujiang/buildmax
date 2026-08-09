package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	wsconn "github.com/gougoujiang/buildmax/internal/server/websocket"
	"github.com/gougoujiang/buildmax/internal/service/conversation"

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
	teamID string

	writeCh chan []byte
	closed  chan struct{}

	turnsMu sync.Mutex
	turns   map[string]*sync.Mutex

	cancel context.CancelFunc
}

// turnLock returns the per-conversation mutex, allocating one on first use.
// Turns in different conversations run independently; turns in the same
// conversation are serialized.
func (wc *wsConn) turnLock(conversationID string) *sync.Mutex {
	wc.turnsMu.Lock()
	defer wc.turnsMu.Unlock()
	if wc.turns == nil {
		wc.turns = make(map[string]*sync.Mutex)
	}
	m, ok := wc.turns[conversationID]
	if !ok {
		m = &sync.Mutex{}
		wc.turns[conversationID] = m
	}
	return m
}

// wsSink implements model.StreamSink by sending conversation.message.delta events.
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
	if h.cfg.TeamStore == nil {
		http.Error(w, "teams not configured", http.StatusServiceUnavailable)
		return
	}
	teamID, ok := pathValueRequired(w, r, "team_id")
	if !ok {
		return
	}
	_, teamID, ok = h.withExplicitTeam(w, r, userID, teamID)
	if !ok {
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
		conn:    conn,
		h:       h,
		userID:  userID,
		teamID:  teamID,
		writeCh: make(chan []byte, wsWriteChSize),
		closed:  make(chan struct{}),
		cancel:  cancel,
	}

	h.connRegistry.Register(userID, wc)

	slog.Info("ws connected", "user_id", userID, "remote", r.RemoteAddr)
	go wc.writeLoop(ctx)
	wc.readLoop(ctx)
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
	conv, err := wc.h.cfg.ConversationStore.CreateConversationInTeam(ctx, wc.teamID, wc.userID, channel, wc.userID)
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
	if conv == nil || conv.TeamID != wc.teamID {
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
	m := wc.turnLock(conversationID)
	if !m.TryLock() {
		slog.Info("ws turn rejected: conversation busy", "user_id", wc.userID, "conversation_id", conversationID)
		wc.sendEvent(wsconn.TypeConversationError, wsconn.ConversationError{
			ConversationID: conversationID,
			Error:          "a conversation turn is already in progress",
		})
		return
	}
	go func() {
		defer m.Unlock()
		wc.executeConversationTurn(ctx, conversationID, message, channel)
	}()
}

// RunSystemConversationTurn runs a system-triggered conversation turn (e.g. task completion).
func (wc *wsConn) RunSystemConversationTurn(ctx context.Context, conversationID, message string) {
	m := wc.turnLock(conversationID)
	m.Lock()
	defer m.Unlock()
	wc.executeConversationTurn(ctx, conversationID, message, conversation.ChannelSystem)
}

func (wc *wsConn) executeConversationTurn(ctx context.Context, conversationID, message, channel string) {
	slog.Info("ws turn start", "user_id", wc.userID, "conversation_id", conversationID, "channel", channel)
	sink := &wsSink{c: wc, conversationID: conversationID}
	svc := wc.h.conversationService()
	_, err := svc.HandleTurn(ctx, conversation.HandleTurnCmd{
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
	wc.sendEvent(wsconn.TypeMessageCompleted, wsconn.MessageCompleted{ConversationID: conversationID})
}

func (wc *wsConn) cleanup() {
	wc.h.connRegistry.Unregister(wc.userID, wc)
	wc.cancel()
	select {
	case <-wc.closed:
	default:
		close(wc.closed)
	}
	close(wc.writeCh)
}
