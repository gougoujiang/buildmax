package work

import (
	"bytes"
	"context"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corespace "github.com/gougoujiang/buildmax/internal/core/space"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

const filesTestSecret = "files-test-secret"

type testPersistStorage struct {
	files map[string]map[string][]byte
}

func newTestPersistStorage() *testPersistStorage {
	return &testPersistStorage{files: make(map[string]map[string][]byte)}
}

func (s *testPersistStorage) Put(ctx context.Context, spaceID string, relPath string, r io.Reader) error {
	if s.files[spaceID] == nil {
		s.files[spaceID] = make(map[string][]byte)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.files[spaceID][relPath] = data
	return nil
}

func (s *testPersistStorage) Get(ctx context.Context, spaceID string, relPath string) ([]byte, error) {
	if s.files[spaceID] == nil {
		return nil, apierr.ErrNotFound
	}
	data, ok := s.files[spaceID][relPath]
	if !ok {
		return nil, apierr.ErrNotFound
	}
	return append([]byte(nil), data...), nil
}

func (s *testPersistStorage) ListFiles(ctx context.Context, spaceID string) ([]string, error) {
	var out []string
	for relPath := range s.files[spaceID] {
		out = append(out, relPath)
	}
	return out, nil
}

func (s *testPersistStorage) MaterializeToDir(ctx context.Context, spaceID string, dstDir string) error {
	return nil
}

func (s *testPersistStorage) PutRunGlobal(ctx context.Context, ref blob.RunObjectRef, r io.Reader) error {
	return nil
}

func (s *testPersistStorage) GetRunGlobal(ctx context.Context, ref blob.RunObjectRef) ([]byte, error) {
	return nil, apierr.ErrNotFound
}

func TestSpaceScopedFilesHandlers(t *testing.T) {
	spaceA := "tm_personal_u1"
	spaceB := "tm_shared_u1"
	persist := newTestPersistStorage()
	persist.files[spaceA] = map[string][]byte{"space-a.txt": []byte("alpha")}
	persist.files[spaceB] = map[string][]byte{"space-b.txt": []byte("beta")}

	h := New(Config{
		JWTSecret:      filesTestSecret,
		Spaces:         &mock.MockSpaceStore{Spaces: []corespace.Space{{ID: spaceA, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}, {ID: spaceB, Name: "Shared", CreatedBy: "u1"}}, Members: []corespace.Member{{SpaceID: spaceA, UserID: "u1", Role: corespace.RoleOwner}, {SpaceID: spaceB, UserID: "u1", Role: corespace.RoleOwner}}},
		PersistStorage: persist,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	token := testsupport.SignJWT("u1", filesTestSecret)

	t.Run("list files uses space scope", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/spaces/"+spaceA+"/files", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "space-a.txt") {
			t.Fatalf("body %q missing space-a.txt", body)
		}
		if strings.Contains(body, "space-b.txt") {
			t.Fatalf("body %q should not include space-b.txt", body)
		}
	})

	t.Run("file content uses space scope", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/spaces/"+spaceB+"/files/space-b.txt", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if got := rec.Body.String(); got != "beta" {
			t.Fatalf("body = %q, want %q", got, "beta")
		}
	})

	t.Run("upload stores file in selected space", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("files", "notes.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("shared-space")); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/spaces/"+spaceB+"/upload", body)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if string(persist.files[spaceB]["notes.txt"]) != "shared-space" {
			t.Fatalf("spaceB notes.txt = %q, want %q", persist.files[spaceB]["notes.txt"], "shared-space")
		}
		if _, ok := persist.files[spaceA]["notes.txt"]; ok {
			t.Fatal("spaceA should not receive uploaded file")
		}
	})

	t.Run("non member cannot access space files", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/spaces/tm_other/files", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}
