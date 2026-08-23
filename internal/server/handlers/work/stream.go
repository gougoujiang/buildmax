package work

import (
	"bufio"
	"net/http"
	"strings"

	"github.com/gougoujiang/buildmax/internal/server/httputil"
	wsconn "github.com/gougoujiang/buildmax/internal/server/websocket"
)

func (h *Handler) getChatStreamHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Tasks, "tasks not configured")
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
	// Flush the headers now. Without this the response is buffered until the
	// first event, so a client watching a quiet run waits for the stream to
	// open instead of being told it is open and then waiting for output.
	if flusher != nil {
		flusher.Flush()
	}

	events, unsub := h.cfg.Hub.Subscribe(taskID)
	defer unsub()

	if buf := h.cfg.Hub.Buffer(taskID); buf != "" {
		writeSSE(w, buf)
		if flusher != nil {
			flusher.Flush()
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.cfg.Drain:
			// This server is going away and the run is not: it lives in the
			// database and keeps streaming into whichever instance the client
			// reconnects to. Saying so is what stops the client from reading a
			// closed connection as a finished run.
			writeSSEEvent(w, streamEventDraining, "")
			if flusher != nil {
				flusher.Flush()
			}
			return
		case msg, ok := <-events:
			if !ok {
				return
			}
			if msg == wsconn.StreamEventDone {
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

// streamEventDraining names the SSE event this server sends before it stops.
// It is a named event rather than a reserved data payload because the data
// frames carry agent output, which can say anything.
const streamEventDraining = "draining"

// writeSSEEvent writes a named event. An empty payload still gets a data line,
// because an event with no data is not delivered by every SSE parser.
func writeSSEEvent(w http.ResponseWriter, event, payload string) {
	_, _ = w.Write([]byte("event: " + event + "\n"))
	writeSSE(w, payload)
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
