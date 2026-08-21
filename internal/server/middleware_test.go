package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	buildmaxlog "github.com/gougoujiang/buildmax/internal/infra/log"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buildmaxlog.Init(buildmaxlog.LogConfig{LogsDir: t.TempDir(), Level: "debug"})
	var buf bytes.Buffer
	buildmaxlog.SetOutput(&buf)
	return &buf
}

func serve(h http.Handler, r *http.Request) (*httptest.ResponseRecorder, *http.Request) {
	w := httptest.NewRecorder()
	requestLoggingMiddleware(h).ServeHTTP(w, r)
	return w, r
}

func TestRequestLogRecordsOutcome(t *testing.T) {
	buf := captureLog(t)
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	})

	serve(h, httptest.NewRequest(http.MethodPost, "/api/teams?limit=5", nil))

	out := buf.String()
	for _, want := range []string{"msg=request", "method=POST", "path=/api/teams", "status=201", "bytes=5", `query="limit=5"`, "duration_ms="} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in %q", want, out)
		}
	}
}

// The id has to reach both the caller and the records the handler wrote, or it
// cannot join a bug report to a log line.
func TestRequestIDIsSharedByHeaderAndRecords(t *testing.T) {
	buf := captureLog(t)
	h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		slog.ErrorContext(r.Context(), "handler error", "handler", "list_teams")
	})

	w, _ := serve(h, httptest.NewRequest(http.MethodGet, "/api/teams", nil))

	id := w.Header().Get(RequestIDHeader)
	if !strings.HasPrefix(id, "rq_") {
		t.Fatalf("no request id in %s header: %q", RequestIDHeader, id)
	}
	out := buf.String()
	if strings.Count(out, "request_id="+id) != 2 {
		t.Errorf("want the handler record and the request line to share request_id=%s, got %q", id, out)
	}
}

func TestRequestLevelFollowsStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{
		{http.StatusOK, "level=INFO"},
		{http.StatusNotFound, "level=WARN"},
		{http.StatusInternalServerError, "level=ERROR"},
	} {
		buf := captureLog(t)
		h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status) })

		serve(h, httptest.NewRequest(http.MethodGet, "/api/teams", nil))

		if out := buf.String(); !strings.Contains(out, tc.want) {
			t.Errorf("status %d: want %s in %q", tc.status, tc.want, out)
		}
	}
}

// A handler that never calls WriteHeader answered 200, not 0.
func TestImplicitStatusIsOK(t *testing.T) {
	buf := captureLog(t)
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("x")) })

	serve(h, httptest.NewRequest(http.MethodGet, "/api/teams", nil))

	if out := buf.String(); !strings.Contains(out, "status=200") {
		t.Errorf("want status=200 in %q", out)
	}
}

// The SSE handlers assert http.Flusher and call the result with no nil check,
// so losing it through the wrapper is a panic on every streamed response.
func TestFlusherSurvivesTheWrapper(t *testing.T) {
	captureLog(t)
	flushed := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Error("handler cannot reach http.Flusher through the middleware")
			return
		}
		_, _ = w.Write([]byte("data: x\n\n"))
		f.Flush()
		flushed = true
	})

	serve(h, httptest.NewRequest(http.MethodGet, "/api/teams/tm_1/tasks/t_1/stream", nil))

	if !flushed {
		t.Error("SSE handler did not complete")
	}
}

// gorilla/websocket upgrades through http.ResponseController, which walks
// Unwrap. httptest's recorder cannot hijack, so this asserts the wrapper does
// not swallow the attempt rather than that the hijack succeeds.
func TestHijackerIsReachable(t *testing.T) {
	captureLog(t)
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(http.Hijacker); !ok {
			t.Error("handler cannot reach http.Hijacker through the middleware")
		}
		if _, _, err := http.NewResponseController(w).Hijack(); err == nil {
			t.Error("expected the test recorder to refuse a hijack")
		}
	})

	serve(h, httptest.NewRequest(http.MethodGet, "/api/teams/tm_1/ws", nil))
}
