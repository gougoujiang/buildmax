package httpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func respond(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// The whole point: the sentence the server wrote has to survive to the caller.
func TestServerMessageSurvives(t *testing.T) {
	err := DecodeError(respond(http.StatusConflict, `{"error":"task has a run already in progress"}`), "worker API PATCH /api/worker/task-runs/r_1")

	got := err.Error()
	for _, want := range []string{"task has a run already in progress", "409", "worker API PATCH"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in %q", want, got)
		}
	}
	if err.Message != "task has a run already in progress" {
		t.Errorf("Message = %q", err.Message)
	}
}

func TestMessageKeyIsAcceptedToo(t *testing.T) {
	err := DecodeError(respond(http.StatusBadGateway, `{"message":"upstream refused"}`), "")

	if err.Message != "upstream refused" {
		t.Errorf("Message = %q, want the message key to be read", err.Message)
	}
}

func TestErrorKeyWinsOverMessage(t *testing.T) {
	err := DecodeError(respond(http.StatusBadRequest, `{"error":"first","message":"second"}`), "")

	if err.Message != "first" {
		t.Errorf("Message = %q, want the error key to win", err.Message)
	}
}

// A proxy answering HTML, or a body that is empty, must degrade to the status
// rather than produce a confusing message or an error about parsing.
func TestUndecodableBodyFallsBackToStatus(t *testing.T) {
	for name, body := range map[string]string{
		"html":  "<html><body>502 Bad Gateway</body></html>",
		"empty": "",
	} {
		err := DecodeError(respond(http.StatusBadGateway, body), "")
		if err.Message != "" {
			t.Errorf("%s: Message = %q, want empty", name, err.Message)
		}
		if got := err.Error(); !strings.Contains(got, "502") {
			t.Errorf("%s: %q should still name the status", name, got)
		}
	}
}

// An error page big enough to be a denial of service is not worth reading.
func TestBodyIsBounded(t *testing.T) {
	huge := `{"error":"` + strings.Repeat("x", 10*maxErrorBody) + `"}`

	err := DecodeError(respond(http.StatusInternalServerError, huge), "")

	if len(err.Message) > maxErrorBody {
		t.Errorf("message length %d exceeds the read cap %d", len(err.Message), maxErrorBody)
	}
}

func TestOpIsOptional(t *testing.T) {
	err := DecodeError(respond(http.StatusForbidden, `{"error":"forbidden"}`), "")

	if got := err.Error(); !strings.HasPrefix(got, "server 403") {
		t.Errorf("without an op the error should lead with the status: %q", got)
	}
}
