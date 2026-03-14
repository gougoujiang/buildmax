package server

import (
	"bufio"
	"net/http"
	"strings"

	"buildmax/internal/streamhub"
)

// getChatStreamHandler handles GET /api/workspaces/{workspace_id}/chats/{chat_id}/stream.
// Returns text/event-stream for the chat (no run_id in URL). Hub is keyed by chat_id.
func (s *Server) getChatStreamHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	chatID := r.PathValue("chat_id")
	if chatID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_id required")
		return
	}
	_, ok = s.getChatForWorkspace(w, r, workspaceID, chatID)
	if !ok {
		return
	}
	if s.hub == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "stream not available")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	events, unsub := s.hub.Subscribe(chatID)
	defer unsub()

	if buf := s.hub.Buffer(chatID); buf != "" {
		writeSSE(w, buf)
		if flusher != nil {
			flusher.Flush()
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-events:
			if !ok {
				return
			}
			if msg == streamhub.StreamEventDone {
				writeSSE(w, "done")
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
			writeSSE(w, msg)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// writeSSE writes one SSE event: each line of payload as "data: line\n", then "\n".
func writeSSE(w http.ResponseWriter, payload string) {
	if payload == "" {
		w.Write([]byte("data: \n\n"))
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(payload))
	for scanner.Scan() {
		w.Write([]byte("data: "))
		w.Write(scanner.Bytes())
		w.Write([]byte("\n"))
	}
	w.Write([]byte("\n"))
}
