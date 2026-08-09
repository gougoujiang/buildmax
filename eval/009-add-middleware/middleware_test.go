package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDMiddleware_HeaderPresent(t *testing.T) {
	handler := RequestID(NewRouter())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if id := rr.Header().Get("X-Request-ID"); id == "" {
		t.Error("X-Request-ID header is missing or empty")
	}
}

func TestRequestIDMiddleware_UniquePerRequest(t *testing.T) {
	handler := RequestID(NewRouter())
	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		id := rr.Header().Get("X-Request-ID")
		if id == "" {
			t.Fatalf("request %d: empty X-Request-ID", i)
		}
		if seen[id] {
			t.Fatalf("duplicate X-Request-ID %q on request %d", id, i)
		}
		seen[id] = true
	}
}

func TestRequestIDMiddleware_PassesThrough(t *testing.T) {
	handler := RequestID(NewRouter())
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if body := rr.Body.String(); body != "hello" {
		t.Errorf("body = %q, want hello", body)
	}
}
