package server

import (
	"bufio"
	"net/http"
	"strings"
)

// getRunStreamHandler handles GET /api/workspaces/{workspace_id}/chats/{chat_id}/runs/{run_id}/stream.
// Returns text/event-stream: existing buffer (if any), then live deltas, then a "done" event when the run completes.
// Requires workspace ownership and that the run belongs to the chat in that workspace.
func (s *Server) getRunStreamHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	chatID := r.PathValue("chat_id")
	runID := r.PathValue("run_id")
	if chatID == "" || runID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_id and run_id required")
		return
	}
	chat, ok := s.getChatForWorkspace(w, r, workspaceID, chatID)
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.ChatRunStore, "chat runs not configured") {
		return
	}
	run, chatFromRun, err := s.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), runID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_run_stream", "run_id", runID)
		return
	}
	if run == nil || chatFromRun == nil || chatFromRun.WorkspaceID != workspaceID || chatFromRun.ChatID != chatID {
		writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	_ = chat // used for auth only
	if s.hub == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "stream not available")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	events, unsub := s.hub.Subscribe(runID)
	defer unsub() // release subscription when handler returns (e.g. client disconnect or done)

	// Send current buffer if any (catch-up for late joiners).
	if buf := s.hub.Buffer(runID); buf != "" {
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
			if msg == StreamEventDone {
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
