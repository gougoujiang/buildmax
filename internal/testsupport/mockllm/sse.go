package mockllm

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// sse writes an event stream. The three protocols disagree about whether an
// event carries a name, so the name is optional and the payload is always a
// "data:" line.
type sse struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func newSSE(w http.ResponseWriter) *sse {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	return &sse{w: w, flusher: flusher}
}

// send writes one event. A named event is what the Anthropic decoder reads;
// the OpenAI protocols ignore the name and read the payload's own type field.
func (s *sse) send(name string, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if name != "" {
		fmt.Fprintf(s.w, "event: %s\n", name)
	}
	fmt.Fprintf(s.w, "data: %s\n\n", encoded)
	s.flush()
}

// done writes the terminator the OpenAI protocols expect. Anthropic ends on
// its own message_stop event and needs none.
func (s *sse) done() {
	fmt.Fprint(s.w, "data: [DONE]\n\n")
	s.flush()
}

func (s *sse) flush() {
	if s.flusher != nil {
		s.flusher.Flush()
	}
}
