package artifact

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	"github.com/gougoujiang/buildmax/internal/mock"
)

// fakeAdmitter answers the storage question and records what it was asked, so a
// test can tell the cheap pre-check from the exact one that knows the size.
type fakeAdmitter struct {
	allow  bool
	reason string
	err    error
	asked  []int64
}

func (f *fakeAdmitter) CheckStorage(_ context.Context, _ string, addBytes int64) (bool, string, error) {
	f.asked = append(f.asked, addBytes)
	if f.err != nil {
		return false, "", f.err
	}
	return f.allow, f.reason, nil
}

func serviceWithQuota(admitter StorageAdmitter) (*Service, *mock.MockArtifactStore, *mock.MockArtifactStorage) {
	store := &mock.MockArtifactStore{}
	storage := mock.NewMockArtifactStorage()
	return &Service{Artifacts: store, Storage: storage, Quota: admitter}, store, storage
}

func upload(s *Service, body string) (*coreartifact.Artifact, error) {
	return s.Create(context.Background(), CreateInput{
		TeamID:        "t_1",
		Filename:      "report.md",
		SourceType:    coreartifact.SourceUserUpload,
		CreatedByType: coreartifact.CreatorUser,
		CreatedByID:   "u_1",
		Content:       strings.NewReader(body),
	})
}

// The size is not known until the body has gone by, so the exact check has to
// happen after the stream. The cheap one in front of it is what stops a full
// team from writing a file to disk on every attempt.
func TestCreateAsksTheQuotaTwiceOnceItKnowsTheSize(t *testing.T) {
	admitter := &fakeAdmitter{allow: true}
	s, _, _ := serviceWithQuota(admitter)

	if _, err := upload(s, "hello"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(admitter.asked) != 2 {
		t.Fatalf("asked = %v, want a probe and an exact check", admitter.asked)
	}
	if admitter.asked[0] != 0 {
		t.Fatalf("first ask = %d, want 0 — the probe adds nothing", admitter.asked[0])
	}
	if admitter.asked[1] != int64(len("hello")) {
		t.Fatalf("second ask = %d, want %d", admitter.asked[1], len("hello"))
	}
}

// A refusal must leave nothing: no record, and no bytes in the bucket that no
// record describes.
func TestCreateLeavesNothingWhenTheQuotaRefuses(t *testing.T) {
	admitter := &fakeAdmitter{allow: false, reason: "quota exceeded: storage limit, 900 of 1000 bytes used"}
	s, store, storage := serviceWithQuota(admitter)

	_, err := upload(s, "hello")
	if err == nil {
		t.Fatal("an over-quota upload succeeded")
	}
	if !errors.Is(err, ErrStorageQuota) {
		t.Fatalf("err = %v, want it to be ErrStorageQuota", err)
	}
	// A quota refusal is a 429, not a 400: the file is fine, the space is full.
	kind, ok := apierr.KindOf(err)
	if !ok || kind != apierr.KindQuotaExceeded {
		t.Fatalf("kind = %v (known %v), want %v", kind, ok, apierr.KindQuotaExceeded)
	}
	if !strings.Contains(err.Error(), "900 of 1000") {
		t.Fatalf("err = %q, want it to carry the quota service's numbers", err.Error())
	}
	if store.Count() != 0 {
		t.Fatalf("records = %d after a refusal, want 0", store.Count())
	}
	if n := storage.ObjectCount(); n != 0 {
		t.Fatalf("objects = %d after a refusal, want 0", n)
	}
}

// The probe refuses before anything is written, so a team that is already full
// does not stream a file to disk on every attempt.
func TestAFullTeamIsRefusedBeforeTheBodyIsRead(t *testing.T) {
	admitter := &fakeAdmitter{allow: false}
	s, _, storage := serviceWithQuota(admitter)

	if _, err := upload(s, "hello"); err == nil {
		t.Fatal("an upload to a full team succeeded")
	}
	if len(admitter.asked) != 1 {
		t.Fatalf("asked = %v, want only the probe", admitter.asked)
	}
	if n := storage.ObjectCount(); n != 0 {
		t.Fatalf("objects = %d, want nothing written", n)
	}
}

// An unreadable quota refuses rather than admits, matching the run and token
// limits: silently accepting unmetered storage is the failure mode worth
// avoiding.
func TestCreateRefusesWhenTheQuotaCannotBeRead(t *testing.T) {
	s, store, _ := serviceWithQuota(&fakeAdmitter{err: errors.New("database down")})

	if _, err := upload(s, "hello"); err == nil {
		t.Fatal("an unreadable quota admitted the upload")
	}
	if store.Count() != 0 {
		t.Fatalf("records = %d, want 0", store.Count())
	}
}

// No quota service means no storage limit. A deployment without one still
// uploads, which is what keeps the capability independent of the quota model.
func TestCreateAdmitsWithoutAQuotaService(t *testing.T) {
	s, store, _ := serviceWithQuota(nil)
	// Assigned as a nil interface rather than a typed nil, which is what the
	// handler's own nil check exists to produce.
	s.Quota = nil

	if _, err := upload(s, "hello"); err != nil {
		t.Fatalf("create without a quota service: %v", err)
	}
	if store.Count() != 1 {
		t.Fatalf("records = %d, want 1", store.Count())
	}
}
