package taskrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/infra/wsarchive"
)

// storedBlob is one payload a fakeCheckpointStore received.
type storedBlob struct {
	spaceID string
	sha     string
	bytes   []byte
}

// fakeCheckpointStore records what a capture uploaded and recomputes the digest
// of the exact bytes it received, so a test can prove the reported digest covers
// the stored bytes.
type fakeCheckpointStore struct {
	puts []storedBlob
}

func (f *fakeCheckpointStore) Put(_ context.Context, spaceID, sha256hex string, src io.Reader) (string, error) {
	raw, err := io.ReadAll(src)
	if err != nil {
		return "", err
	}
	f.puts = append(f.puts, storedBlob{spaceID: spaceID, sha: sha256hex, bytes: raw})
	return "key/" + sha256hex, nil
}

// seedRecorder is a worker API stand-in that records the seed finalize request
// and, optionally, answers the base lookup with an existing base.
type seedRecorder struct {
	hasBase  bool
	finalize *workerclient.SeedCheckpointRequest
}

func (s *seedRecorder) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/workspace-base") && r.Method == http.MethodGet:
			if !s.hasBase {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			_ = json.NewEncoder(w).Encode(workerclient.WorkspaceBaseResponse{
				CheckpointID: "wc_existing", PayloadFormat: wsarchive.PayloadFormat,
				PayloadSHA256: strings.Repeat("a", 64), SizeBytes: 1,
			})
		case strings.HasSuffix(r.URL.Path, "/workspace-checkpoints") && r.Method == http.MethodPost:
			var req workerclient.SeedCheckpointRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			s.finalize = &req
			_ = json.NewEncoder(w).Encode(workerclient.SeedCheckpointResponse{CheckpointID: "wc_seed"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func writeWorkspace(t *testing.T) string {
	t.Helper()
	ws := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(filepath.Join(ws, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "top.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "sub", "nested.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ws
}

// TestCaptureAndFinalizeSeed_UploadsBytesThenRecordsMatchingDigest pins the
// commit protocol: the bytes reach the store first, and the digest the finalize
// records is the digest of exactly those bytes.
func TestCaptureAndFinalizeSeed_UploadsBytesThenRecordsMatchingDigest(t *testing.T) {
	ctx := context.Background()
	ws := writeWorkspace(t)
	staging := filepath.Join(t.TempDir(), "staging")

	rec := &seedRecorder{}
	srv := rec.server(t)
	defer srv.Close()
	store := &fakeCheckpointStore{}
	cfg := workerclient.WorkerAPIClientConfig{BaseURL: srv.URL, Token: "t", Client: srv.Client()}

	if err := captureAndFinalizeSeed(ctx, store, cfg, staging, ws, "sp_1", "rt_1"); err != nil {
		t.Fatalf("captureAndFinalizeSeed: %v", err)
	}

	if len(store.puts) != 1 {
		t.Fatalf("expected exactly one payload upload, got %d", len(store.puts))
	}
	put := store.puts[0]
	if put.spaceID != "sp_1" {
		t.Fatalf("payload uploaded for space %q, want sp_1", put.spaceID)
	}
	// The digest the caller passed to Put must be the digest of the bytes Put
	// actually received — the tee, not a value computed from something else.
	sum := sha256.Sum256(put.bytes)
	want := hex.EncodeToString(sum[:])
	if put.sha != want {
		t.Fatalf("upload digest %q does not match its bytes %q", put.sha, want)
	}
	if rec.finalize == nil {
		t.Fatal("seed was never finalized")
	}
	if rec.finalize.PayloadSHA256 != want {
		t.Fatalf("finalize digest %q != uploaded digest %q", rec.finalize.PayloadSHA256, want)
	}
	if rec.finalize.PayloadFormat != wsarchive.PayloadFormat {
		t.Fatalf("finalize format %q, want %q", rec.finalize.PayloadFormat, wsarchive.PayloadFormat)
	}
	if rec.finalize.SizeBytes != int64(len(put.bytes)) {
		t.Fatalf("finalize size %d != uploaded byte count %d", rec.finalize.SizeBytes, len(put.bytes))
	}
	// top.txt, sub/, sub/nested.txt.
	if rec.finalize.EntryCount != 3 {
		t.Fatalf("finalize entry count %d, want 3", rec.finalize.EntryCount)
	}

	// The uploaded archive round-trips to the same tree.
	dest := filepath.Join(t.TempDir(), "restored")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := wsarchive.Extract(bytes.NewReader(put.bytes), dest, checkpointLimits); err != nil {
		t.Fatalf("uploaded archive does not extract: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "sub", "nested.txt"))
	if err != nil || string(got) != "world" {
		t.Fatalf("restored nested file = %q, err %v", got, err)
	}
}

// TestCaptureAndFinalizeSeed_StagesArchiveOutsideWorkspace pins §12.1: the
// archive is never written inside the tree it captures, and its staging is
// cleaned up.
func TestCaptureAndFinalizeSeed_StagesArchiveOutsideWorkspace(t *testing.T) {
	ctx := context.Background()
	ws := writeWorkspace(t)
	staging := filepath.Join(t.TempDir(), "staging")

	rec := &seedRecorder{}
	srv := rec.server(t)
	defer srv.Close()
	cfg := workerclient.WorkerAPIClientConfig{BaseURL: srv.URL, Token: "t", Client: srv.Client()}

	if err := captureAndFinalizeSeed(ctx, &fakeCheckpointStore{}, cfg, staging, ws, "sp_1", "rt_1"); err != nil {
		t.Fatalf("captureAndFinalizeSeed: %v", err)
	}

	entries, err := os.ReadDir(ws)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "seed-") {
			t.Fatalf("archive staged inside the captured workspace: %s", e.Name())
		}
	}
	// The staging directory holds no leftover archive after a successful capture.
	staged, err := os.ReadDir(staging)
	if err != nil {
		t.Fatal(err)
	}
	if len(staged) != 0 {
		t.Fatalf("staging dir should be empty after capture, has %d entries", len(staged))
	}
}

// TestSeedWorkspaceIfFirstRun_SkipsWhenBaseExists pins that a run with a base
// does not seed — seeding is the first run's job, and a continuing run restores
// instead.
func TestSeedWorkspaceIfFirstRun_SkipsWhenBaseExists(t *testing.T) {
	ctx := context.Background()
	ws := writeWorkspace(t)

	rec := &seedRecorder{hasBase: true}
	srv := rec.server(t)
	defer srv.Close()
	store := &fakeCheckpointStore{}
	dirs := runDirs{runDir: t.TempDir(), runWorkspace: ws}
	input := RunTaskInput{
		Checkpoints: store,
		WorkerAPI:   workerclient.WorkerAPIClientConfig{BaseURL: srv.URL, Token: "t", Client: srv.Client()},
	}
	task := &coretask.Task{ID: "t1", SpaceID: "sp_1"}
	run := &coretask.Run{ID: "rt_1"}

	if err := seedWorkspaceIfFirstRun(ctx, input, task, run, dirs); err != nil {
		t.Fatalf("seedWorkspaceIfFirstRun: %v", err)
	}
	if len(store.puts) != 0 {
		t.Fatalf("a run with a base must not seed, but uploaded %d payloads", len(store.puts))
	}
	if rec.finalize != nil {
		t.Fatal("a run with a base must not finalize a seed")
	}
}

// TestSeedWorkspaceIfFirstRun_SkipsWhenServerHasNoCheckpointRoute pins that a
// server that does not run the checkpoint contract (an evaluation control plane,
// which answers 404) makes the run seed nothing rather than fail closed on a
// missing route.
func TestSeedWorkspaceIfFirstRun_SkipsWhenServerHasNoCheckpointRoute(t *testing.T) {
	ctx := context.Background()
	// A control plane that knows no checkpoint route: every path 404s.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	store := &fakeCheckpointStore{}
	input := RunTaskInput{
		Checkpoints: store,
		WorkerAPI:   workerclient.WorkerAPIClientConfig{BaseURL: srv.URL, Token: "t", Client: srv.Client()},
	}
	dirs := runDirs{runDir: t.TempDir(), runWorkspace: writeWorkspace(t)}
	task := &coretask.Task{ID: "t1", SpaceID: "sp_1"}
	run := &coretask.Run{ID: "rt_1"}

	if err := seedWorkspaceIfFirstRun(ctx, input, task, run, dirs); err != nil {
		t.Fatalf("a server with no checkpoint route should be a no-op, got %v", err)
	}
	if len(store.puts) != 0 {
		t.Fatalf("must not seed against a server with no checkpoint route, uploaded %d", len(store.puts))
	}
}

// TestSeedWorkspaceIfFirstRun_NoopWithoutCheckpointStore pins that a deployment
// without checkpoint storage (a CLI or eval run) seeds nothing and does not fail.
func TestSeedWorkspaceIfFirstRun_NoopWithoutCheckpointStore(t *testing.T) {
	ctx := context.Background()
	dirs := runDirs{runDir: t.TempDir(), runWorkspace: writeWorkspace(t)}
	input := RunTaskInput{} // no Checkpoints, no WorkerAPI
	task := &coretask.Task{ID: "t1", SpaceID: "sp_1"}
	run := &coretask.Run{ID: "rt_1"}

	if err := seedWorkspaceIfFirstRun(ctx, input, task, run, dirs); err != nil {
		t.Fatalf("seedWorkspaceIfFirstRun without a store should be a no-op, got %v", err)
	}
}
