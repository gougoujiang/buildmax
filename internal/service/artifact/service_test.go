package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	"github.com/gougoujiang/buildmax/internal/mock"

	"github.com/gougoujiang/buildmax/internal/util"
)

func newService(t *testing.T) (*Service, *mock.MockArtifactStore, *mock.MockArtifactStorage) {
	t.Helper()
	store := &mock.MockArtifactStore{}
	storage := mock.NewMockArtifactStorage()
	return &Service{Artifacts: store, Storage: storage}, store, storage
}

func createFile(t *testing.T, s *Service, teamID, filename, body string) *coreartifact.Artifact {
	t.Helper()
	rec, err := s.Create(context.Background(), CreateInput{
		TeamID:        teamID,
		Filename:      filename,
		SourceType:    coreartifact.SourceUserUpload,
		CreatedByType: coreartifact.CreatorUser,
		CreatedByID:   "u_1",
		Content:       strings.NewReader(body),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return rec
}

// The digest and size must describe the bytes that went past, because that is
// the whole claim the record makes about what was stored.
func TestCreateMeasuresTheStream(t *testing.T) {
	svc, _, _ := newService(t)
	body := "the quick brown fox"
	rec := createFile(t, svc, "tm_1", "notes.txt", body)

	if rec.SizeBytes != int64(len(body)) {
		t.Errorf("size = %d, want %d", rec.SizeBytes, len(body))
	}
	sum := sha256.Sum256([]byte(body))
	if rec.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("sha256 = %q, want the digest of the content", rec.SHA256)
	}
	if _, ok := util.CanonicalPublicID(rec.ID); !ok {
		t.Errorf("artifact id = %q, want a canonical public ID", rec.ID)
	}
	if rec.MediaType != "text/plain; charset=utf-8" {
		t.Errorf("media type = %q, want it derived from the extension", rec.MediaType)
	}
}

// A caller's path is not a place the server should be persuaded to reason
// about: one artifact is one file, whatever the name claims.
func TestCreateReducesFilenameToOneElement(t *testing.T) {
	svc, _, _ := newService(t)
	for _, given := range []string{"../../etc/passwd", "dir/sub/report.md", `windows\\dir\\report.md`} {
		rec := createFile(t, svc, "tm_1", given, "x")
		if strings.ContainsAny(rec.Filename, `/\`) {
			t.Errorf("filename for %q kept a path separator: %q", given, rec.Filename)
		}
	}
}

func TestCreateRejectsAnEmptyName(t *testing.T) {
	svc, store, storage := newService(t)
	_, err := svc.Create(context.Background(), CreateInput{
		TeamID:   "tm_1",
		Filename: "   ",
		Content:  strings.NewReader("x"),
	})
	if !errors.Is(err, ErrNoFilename) {
		t.Fatalf("err = %v, want ErrNoFilename", err)
	}
	if store.Count() != 0 || storage.ObjectCount() != 0 {
		t.Error("a rejected upload must leave neither a record nor an object")
	}
}

// The cap is measured, not declared: an oversized stream is detected as it goes
// by rather than trusted from a header.
func TestCreateRefusesContentOverTheLimit(t *testing.T) {
	svc, store, storage := newService(t)
	svc.MaxFileBytes = 8

	_, err := svc.Create(context.Background(), CreateInput{
		TeamID:   "tm_1",
		Filename: "big.bin",
		Content:  strings.NewReader(strings.Repeat("a", 9)),
	})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if store.Count() != 0 {
		t.Error("an oversized upload must not be recorded")
	}
	if storage.ObjectCount() != 0 {
		t.Error("an oversized upload must not leave its bytes behind")
	}
}

func TestCreateAcceptsContentExactlyAtTheLimit(t *testing.T) {
	svc, _, _ := newService(t)
	svc.MaxFileBytes = 8
	rec := createFile(t, svc, "tm_1", "exact.bin", strings.Repeat("a", 8))
	if rec.SizeBytes != 8 {
		t.Errorf("size = %d, want 8 — the limit is inclusive", rec.SizeBytes)
	}
}

func TestCreateRejectsEmptyContent(t *testing.T) {
	svc, store, storage := newService(t)
	_, err := svc.Create(context.Background(), CreateInput{
		TeamID:   "tm_1",
		Filename: "empty.txt",
		Content:  strings.NewReader(""),
	})
	if !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("err = %v, want ErrEmptyContent", err)
	}
	if store.Count() != 0 || storage.ObjectCount() != 0 {
		t.Error("an empty upload must leave nothing behind")
	}
}

// The ordering contract: content is durable before metadata is committed, and a
// record that fails to commit takes its object with it. Otherwise a later
// listing would name bytes nobody can account for.
func TestCreateRemovesContentWhenTheRecordFails(t *testing.T) {
	svc, store, storage := newService(t)
	store.CreateErr = errors.New("database is down")

	_, err := svc.Create(context.Background(), CreateInput{
		TeamID:   "tm_1",
		Filename: "report.md",
		Content:  strings.NewReader("hello"),
	})
	if err == nil {
		t.Fatal("create must report the failure")
	}
	if storage.ObjectCount() != 0 {
		t.Error("a record that never committed must not leave its object behind")
	}
}

func TestCreateReportsStorageFailureWithoutRecording(t *testing.T) {
	svc, store, storage := newService(t)
	storage.PutErr = errors.New("bucket is unreachable")

	if _, err := svc.Create(context.Background(), CreateInput{
		TeamID:   "tm_1",
		Filename: "report.md",
		Content:  strings.NewReader("hello"),
	}); err == nil {
		t.Fatal("create must report the failure")
	}
	if store.Count() != 0 {
		t.Error("storage failure must not produce an artifact record")
	}
}

func TestOpenReturnsTheStoredContent(t *testing.T) {
	svc, _, _ := newService(t)
	rec := createFile(t, svc, "tm_1", "notes.txt", "hello")

	body, err := svc.Open(context.Background(), rec)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = body.Close() }()
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want hello", got)
	}
}

// The tombstone is what makes deletion take effect at the boundary, so it has
// to be immediate and it has to be what Get sees.
func TestDeleteHidesTheArtifactImmediately(t *testing.T) {
	svc, _, storage := newService(t)
	rec := createFile(t, svc, "tm_1", "notes.txt", "hello")

	if err := svc.Delete(context.Background(), rec, coreartifact.CreatorUser, "u_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(context.Background(), rec.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete = %v, want ErrNotFound", err)
	}
	items, total, err := svc.List(context.Background(), "tm_1", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("list after delete = %d items (total %d), want none", len(items), total)
	}
	// The object stays: reclaiming it is retention's job, and the boundary has
	// already stopped serving it.
	if storage.ObjectCount() != 1 {
		t.Errorf("objects = %d, want the content still present after a tombstone", storage.ObjectCount())
	}
}

func TestDeleteTwiceReportsNotFound(t *testing.T) {
	svc, _, _ := newService(t)
	rec := createFile(t, svc, "tm_1", "notes.txt", "hello")
	if err := svc.Delete(context.Background(), rec, coreartifact.CreatorUser, "u_1"); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := svc.Delete(context.Background(), rec, coreartifact.CreatorUser, "u_1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete = %v, want ErrNotFound", err)
	}
}

// Half a capability is not a capability: metadata without content, or content
// without a record, cannot answer for an artifact.
func TestAvailableRequiresBothHalves(t *testing.T) {
	if (&Service{Artifacts: &mock.MockArtifactStore{}}).Available() {
		t.Error("a service with no storage must not report itself available")
	}
	if (&Service{Storage: mock.NewMockArtifactStorage()}).Available() {
		t.Error("a service with no store must not report itself available")
	}
	var nilService *Service
	if nilService.Available() {
		t.Error("a nil service must not report itself available")
	}
}

func TestUnconfiguredServiceRefusesEveryCall(t *testing.T) {
	svc := &Service{}
	if _, err := svc.Create(context.Background(), CreateInput{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("create = %v, want ErrNotConfigured", err)
	}
	if _, err := svc.Get(context.Background(), "ar_x"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("get = %v, want ErrNotConfigured", err)
	}
	if _, _, err := svc.List(context.Background(), "tm_1", 10, 0); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("list = %v, want ErrNotConfigured", err)
	}
}

func TestMediaTypeFallsBackForUnknownExtensions(t *testing.T) {
	if got := mediaTypeFor("mystery.zzz"); got != FallbackMediaType {
		t.Errorf("media type = %q, want %q", got, FallbackMediaType)
	}
	if got := mediaTypeFor("noextension"); got != FallbackMediaType {
		t.Errorf("media type = %q, want %q", got, FallbackMediaType)
	}
}
