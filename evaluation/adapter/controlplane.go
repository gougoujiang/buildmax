package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
)

// controlPlane serves the worker API for exactly one run.
//
// A worker reaches its server over HTTP and nothing else: it fetches the run,
// reports status, streams output, and polls for cancellation. So evaluating one
// needs a server that speaks that protocol — not a database, a Portal user, a
// team, or a scheduler. This is the same move mockllm makes for the model side,
// and it is what keeps a worker trial a black-box run of the shipped binary
// rather than an in-process call to the runtime it happens to use.
//
// The request and response bodies are the workerclient types rather than
// hand-written JSON, so a change to the contract breaks this at compile time
// instead of at the first trial that cannot decode a reply.
type controlPlane struct {
	listener net.Listener
	server   *http.Server
	token    string

	mu sync.Mutex
	// response is what GET returns, including the cancel flag a later Cancel
	// flips.
	response workerclient.GetTaskRunResponse
	// patches are the status reports the worker sent, in order. The last one is
	// the run's outcome.
	patches []workerclient.PatchTaskRunRequest
	deltas  []string
	// unauthorized counts requests that arrived without the run token. A worker
	// that reported without one would be a real finding, so it is recorded
	// rather than merely refused.
	unauthorized int
}

// startControlPlane serves the worker API on a loopback port until Close.
func startControlPlane(token string, response workerclient.GetTaskRunResponse) (*controlPlane, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	cp := &controlPlane{listener: listener, token: token, response: response}

	mux := http.NewServeMux()
	base := "/api/worker/task-runs/" + response.Run.ID
	mux.HandleFunc(base, cp.handleRun)
	mux.HandleFunc(base+"/stream", cp.handleStream)

	cp.server = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := cp.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// Nothing here can report; the trial fails on the worker's own
			// error, which is the evidence a reader needs anyway.
			_ = err
		}
	}()
	return cp, nil
}

// URL is the base a worker's server_url points at.
func (c *controlPlane) URL() string { return "http://" + c.listener.Addr().String() }

// Close stops serving.
func (c *controlPlane) Close() {
	_ = c.server.Close()
	_ = c.listener.Close()
}

func (c *controlPlane) handleRun(w http.ResponseWriter, r *http.Request) {
	if !c.authorized(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		c.mu.Lock()
		body := c.response
		c.mu.Unlock()
		writeJSON(w, body)
	case http.MethodPatch:
		var patch workerclient.PatchTaskRunRequest
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		c.mu.Lock()
		c.patches = append(c.patches, patch)
		// The worker refuses to run a run that is not SCHEDULED, and it polls
		// this same endpoint for cancellation, so the served status has to
		// follow what it reported. Leaving it SCHEDULED would describe a run
		// that never started.
		c.response.Run.Status = patch.Status
		c.mu.Unlock()
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (c *controlPlane) handleStream(w http.ResponseWriter, r *http.Request) {
	if !c.authorized(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var req workerclient.StreamDeltaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	c.deltas = append(c.deltas, req.Delta)
	c.mu.Unlock()
	writeJSON(w, map[string]string{"status": "ok"})
}

// authorized checks the run token. A worker holds one credential and it names
// one run, so anything arriving without it is not this trial's worker.
func (c *controlPlane) authorized(r *http.Request) bool {
	if r.Header.Get("Authorization") == "Bearer "+c.token {
		return true
	}
	c.mu.Lock()
	c.unauthorized++
	c.mu.Unlock()
	return false
}

// Cancel makes the next poll report that this run was asked to stop.
func (c *controlPlane) Cancel() {
	c.mu.Lock()
	c.response.Run.CancelRequested = true
	c.mu.Unlock()
}

// Outcome returns the last status the worker reported, and whether it reported
// at all. A worker that exited without reporting is a different fact from one
// that reported a failure.
func (c *controlPlane) Outcome() (workerclient.PatchTaskRunRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.patches) == 0 {
		return workerclient.PatchTaskRunRequest{}, false
	}
	return c.patches[len(c.patches)-1], true
}

// Reports returns every status the worker sent, in order.
func (c *controlPlane) Reports() []workerclient.PatchTaskRunRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]workerclient.PatchTaskRunRequest(nil), c.patches...)
}

// Unauthorized reports how many requests arrived without the run token.
func (c *controlPlane) Unauthorized() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.unauthorized
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
