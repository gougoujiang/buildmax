package handlers

import (
	"bufio"
	"net/http"
	"strings"

	"buildmax/internal/server/httputil"
	streamhub "buildmax/internal/server/websocket"
)

func (h *Handler) getChatStreamHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TaskStore, "tasks not configured")
	if !ok {
		return
	}
	taskID := r.PathValue("task_id")
	if taskID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_id required")
		return
	}
	_, _, ok = h.getTaskForTeam(w, r, teamID, taskID)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	events, unsub := h.hub.Subscribe(taskID)
	defer unsub()

	if buf := h.hub.Buffer(taskID); buf != "" {
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
		_, _ = w.Write([]byte("data: \n\n"))
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(payload))
	for scanner.Scan() {
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(scanner.Bytes())
		_, _ = w.Write([]byte("\n"))
	}
	_, _ = w.Write([]byte("\n"))
}
