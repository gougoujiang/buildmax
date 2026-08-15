package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const responseText = "deployment smoke ok"

type chatRequest struct {
	Stream bool `json:"stream"`
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		resp, err := http.Get("http://127.0.0.1:8080/healthz")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		_ = resp.Body.Close()
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("POST /v1/chat/completions", chatCompletion)
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func chatCompletion(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.Stream {
		writeStream(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      "buildmax-smoke",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "buildmax-smoke",
		"choices": []any{map[string]any{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": responseText},
			"finish_reason": "stop",
		}},
		"usage": map[string]int{"prompt_tokens": 3, "completion_tokens": 3, "total_tokens": 6},
	})
}

func writeStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	created := time.Now().Unix()
	chunk := fmt.Sprintf(`{"id":"buildmax-smoke","object":"chat.completion.chunk","created":%d,"model":"buildmax-smoke","choices":[{"index":0,"delta":{"role":"assistant","content":"%s"},"finish_reason":null}]}`, created, responseText)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
	finish := fmt.Sprintf(`{"id":"buildmax-smoke","object":"chat.completion.chunk","created":%d,"model":"buildmax-smoke","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":3,"total_tokens":6}}`, created)
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", finish)
}
