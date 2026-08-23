package work

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

const (
	streamTestSecret = "test-secret"
	streamTestUser   = "user-1"
	streamTestTeam   = "tm_personal_user1"
	streamTestConv   = "conv-1"
	streamTestTask   = "task-1"
)

// openTaskStream serves a handler wired with drain and returns the open stream
// response. A nil drain is the case an embedded server or a test has.
func openTaskStream(t *testing.T, drain <-chan struct{}) *http.Response {
	t.Helper()
	h := New(Config{
		JWTSecret: streamTestSecret,
		Teams: &mock.MockTeamStore{
			Teams:   []model.Team{{ID: streamTestTeam, Name: "My Space", PersonalForUserID: util.Ptr(streamTestUser), CreatedBy: streamTestUser}},
			Members: []model.TeamMember{{TeamID: streamTestTeam, UserID: streamTestUser, Role: model.TeamRoleOwner}},
		},
		Conversations: &mock.MockConversationStore{
			Conversations: []model.Conversation{{ID: streamTestConv, UserID: streamTestUser, TeamID: streamTestTeam, Channel: "portal", CreatedBy: streamTestUser, CreatedAt: time.Unix(1, 0).UTC()}},
		},
		Tasks: &mock.MockTaskStore{
			List: []model.Task{{ID: streamTestTask, ConversationID: streamTestConv, TeamID: streamTestTeam, Status: "RUNNING", Input: "in", CreatedBy: streamTestUser, CreatedAt: time.Unix(1, 0).UTC()}},
		},
		Drain: drain,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	// Registered after ts.Close so it runs before it: httptest.Server.Close
	// waits for outstanding requests, and a stream that never drains is exactly
	// that. Cancelling the request is what a browser closing the tab does.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/teams/"+streamTestTeam+"/tasks/"+streamTestTask+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(streamTestUser, streamTestSecret))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("stream status = %d, want 200 (%s)", resp.StatusCode, b)
	}
	return resp
}

// readBody reads the rest of the stream on a goroutine, so a test can assert
// that it has *not* ended yet.
func readBody(resp *http.Response) <-chan string {
	body := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(resp.Body)
		body <- string(b)
	}()
	return body
}

// TestTaskStreamEndsOnDrain is the case that used to hold a shutdown open for
// its whole budget: a Portal tab watching a run keeps the connection alive, and
// http.Server.Shutdown never sees it go idle.
func TestTaskStreamEndsOnDrain(t *testing.T) {
	drain := make(chan struct{})
	body := readBody(openTaskStream(t, drain))

	// Nothing is streaming, so the handler is parked exactly where a real
	// watcher would be.
	select {
	case got := <-body:
		t.Fatalf("stream ended before the drain: %q", got)
	case <-time.After(50 * time.Millisecond):
	}

	close(drain)

	select {
	case got := <-body:
		if !strings.Contains(got, "event: draining") {
			t.Errorf("stream body = %q, want a draining event", got)
		}
		if strings.Contains(got, "data: done") {
			t.Errorf("stream body = %q, must not report the run as done", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not end after the drain")
	}
}

// TestTaskStreamWithoutDrainStillServes covers the nil channel a test or an
// embedded server has: it must park, not return immediately.
func TestTaskStreamWithoutDrainStillServes(t *testing.T) {
	body := readBody(openTaskStream(t, nil))
	select {
	case got := <-body:
		t.Fatalf("stream ended with no drain channel: %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}
