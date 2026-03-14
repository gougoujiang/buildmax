package portal

import (
	"bufio"
	"net/http"
	"strings"

	"buildmax/internal/server/httputil"
	"buildmax/internal/streamhub"
)

func (h *Handler) getChatStreamHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	chatID := r.PathValue("chat_id")
	if chatID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "chat_id required")
		return
	}
	_, ok = h.getChatForWorkspace(w, r, workspaceID, chatID)
	if !ok {
		return
	}
	if h.cfg.Hub == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "stream not available")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	events, unsub := h.cfg.Hub.Subscribe(chatID)
	defer unsub()

	if buf := h.cfg.Hub.Buffer(chatID); buf != "" {
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
