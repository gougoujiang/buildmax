package work

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	coreconv "github.com/icloudbb/buildmax/internal/core/conversation"
	corespace "github.com/icloudbb/buildmax/internal/core/space"
	coretask "github.com/icloudbb/buildmax/internal/core/task"
	infra "github.com/icloudbb/buildmax/internal/infra/coordination"
	"github.com/icloudbb/buildmax/internal/mock"
	servercoord "github.com/icloudbb/buildmax/internal/server/coordination"
	wsconn "github.com/icloudbb/buildmax/internal/server/websocket"
	"github.com/icloudbb/buildmax/internal/testsupport"
	"github.com/icloudbb/buildmax/internal/util"
)

// serveTaskStreamReplica stands up one server replica: a work handler wired with
// the given stream hub, serving the SSE route on its own httptest listener. It
// returns the open stream response.
func serveTaskStreamReplica(t *testing.T, hub wsconn.StreamHub) *http.Response {
	t.Helper()
	h := New(Config{
		JWTSecret: streamTestSecret,
		Hub:       hub,
		Spaces: &mock.MockSpaceStore{
			Spaces:  []corespace.Space{{ID: streamTestSpace, Name: "My Space", PersonalForUserID: util.Ptr(streamTestUser), CreatedBy: streamTestUser}},
			Members: []corespace.Member{{SpaceID: streamTestSpace, UserID: streamTestUser, Role: corespace.RoleOwner}},
		},
		Conversations: &mock.MockConversationStore{
			Conversations: []coreconv.Conversation{{ID: streamTestConv, UserID: streamTestUser, SpaceID: streamTestSpace, Channel: "portal", CreatedBy: streamTestUser, CreatedAt: time.Unix(1, 0).UTC()}},
		},
		Tasks: &mock.MockTaskStore{
			List: []coretask.Task{{ID: streamTestTask, ConversationID: streamTestConv, SpaceID: streamTestSpace, Status: "RUNNING", Input: "in", CreatedBy: streamTestUser, CreatedAt: time.Unix(1, 0).UTC()}},
		},
		TaskRuns: &mock.MockTaskRunStore{
			Runs: []coretask.Run{{ID: streamTestRun, TaskID: streamTestTask, Status: "RUNNING", Input: "in", CreatedBy: streamTestUser, CreatedAt: time.Unix(1, 0).UTC()}},
		},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/spaces/"+streamTestSpace+"/tasks/"+streamTestTask+"/stream", nil)
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

// awaitSSEContains reads the SSE body until it contains want or times out.
func awaitSSEContains(t *testing.T, resp *http.Response, want string) {
	t.Helper()
	seen := make(chan struct{})
	go func() {
		var acc []byte
		buf := make([]byte, 256)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				acc = append(acc, buf[:n]...)
				if bytes.Contains(acc, []byte(want)) {
					close(seen)
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-seen:
	case <-time.After(3 * time.Second):
		t.Fatalf("SSE stream never delivered %q", want)
	}
}

// TestTaskStreamCrossesReplicas is the two-replica operating check for the stream
// hub. A worker pushes a delta into replica A's hub — exactly what the worker
// stream route does with Hub.Append — while a Portal client's SSE connection is
// served by a *different* replica B. With coordination.mode redis the two hubs
// share one Redis, so the delta reaches the client on the other replica. Before
// this backend existed, replica B's in-memory hub never saw it.
func TestTaskStreamCrossesReplicas(t *testing.T) {
	mr := miniredis.RunT(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	backendA, err := infra.New(ctx, infra.Options{Address: mr.Addr()})
	if err != nil {
		t.Fatalf("backend A: %v", err)
	}
	t.Cleanup(func() { _ = backendA.Close() })
	backendB, err := infra.New(ctx, infra.Options{Address: mr.Addr()})
	if err != nil {
		t.Fatalf("backend B: %v", err)
	}
	t.Cleanup(func() { _ = backendB.Close() })

	hubA := servercoord.NewStreamHub(ctx, backendA)
	hubB := servercoord.NewStreamHub(ctx, backendB)

	// Replica B serves the client's SSE stream.
	resp := serveTaskStreamReplica(t, hubB)
	// Let the SSE handler's Subscribe register before the delta is pushed.
	time.Sleep(100 * time.Millisecond)

	// Replica A's worker pushes a delta.
	hubA.Append(streamTestRun, "hello-from-replica-A")

	awaitSSEContains(t, resp, "hello-from-replica-A")
}

// TestTaskStreamDoesNotCrossReplicasWithoutCoordination is the contrast: two
// replicas each with their own in-memory hub — the pre-coordination default —
// do not share a stream, so a delta pushed on replica A never reaches a client
// served by replica B. This is the split the coordination backend closes.
func TestTaskStreamDoesNotCrossReplicasWithoutCoordination(t *testing.T) {
	hubA := wsconn.NewStreamHub()
	hubB := wsconn.NewStreamHub()

	resp := serveTaskStreamReplica(t, hubB)
	time.Sleep(100 * time.Millisecond)

	hubA.Append(streamTestRun, "hello-from-replica-A")

	// The delta must NOT arrive on replica B within a generous window.
	seen := make(chan struct{})
	go func() {
		var acc []byte
		buf := make([]byte, 256)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				acc = append(acc, buf[:n]...)
				if bytes.Contains(acc, []byte("hello-from-replica-A")) {
					close(seen)
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-seen:
		t.Fatal("a delta on replica A reached replica B with no shared coordination; the hubs are not actually independent")
	case <-time.After(500 * time.Millisecond):
		// expected: process-local hubs do not share the stream
	}
}
