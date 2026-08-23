package handlers

import (
	"net/http"

	"github.com/gougoujiang/buildmax/internal/server/access"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	wsconn "github.com/gougoujiang/buildmax/internal/server/websocket"
)

// wsUpgradeHandler authenticates the upgrade and hands the socket over.
//
// The credential arrives as a query parameter rather than a header because a
// browser cannot set one on a WebSocket upgrade. Everything after "who is this
// and which team" belongs to internal/server/websocket.
func (h *Handler) wsUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	// A socket opened now would be hijacked past the shutdown that is already
	// running, and its first turn refused anyway. The Portal reconnects with
	// backoff, so refusing sends it to an instance that will still be here.
	if h.draining() {
		http.Error(w, "this server is shutting down", http.StatusServiceUnavailable)
		return
	}
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "token required", http.StatusUnauthorized)
		return
	}
	userID, ok := access.UserIDFromToken(tokenStr, h.cfg.JWTSecret)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.cfg.TeamStore == nil {
		http.Error(w, "teams not configured", http.StatusServiceUnavailable)
		return
	}
	teamID, ok := httputil.PathValue(w, r, "team_id")
	if !ok {
		return
	}
	if _, teamID, ok = h.guard().ExplicitTeam(w, r, userID, teamID); !ok {
		return
	}
	wsconn.Serve(w, r, userID, teamID, h.connDeps())
}

func (h *Handler) connDeps() wsconn.ConnDeps {
	return wsconn.ConnDeps{
		Conversations: h.cfg.ConversationStore,
		Turns:         h.turns,
		Turner:        h.conversationService(),
		Registry:      h.connRegistry,
		CORSOrigin:    h.cfg.CORSOrigin,
	}
}
