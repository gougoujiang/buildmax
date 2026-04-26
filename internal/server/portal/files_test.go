package portal

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
	"buildmax/internal/testutil"
)

const filesTestSecret = "files-test-secret"

type testPersistStorage struct {
	files map[string]map[string][]byte
}

func newTestPersistStorage() *testPersistStorage {
	return &testPersistStorage{files: make(map[string]map[string][]byte)}
}

func (s *testPersistStorage) Put(ctx context.Context, teamID string, relPath string, r io.Reader) error {
	if s.files[teamID] == nil {
		s.files[teamID] = make(map[string][]byte)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.files[teamID][relPath] = data
	return nil
}

func (s *testPersistStorage) Get(ctx context.Context, teamID string, relPath string) ([]byte, error) {
	if s.files[teamID] == nil {
		return nil, blob.ErrNotFound
	}
	data, ok := s.files[teamID][relPath]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return append([]byte(nil), data...), nil
}

func (s *testPersistStorage) ListFiles(ctx context.Context, teamID string) ([]string, error) {
	var out []string
	for relPath := range s.files[teamID] {
		out = append(out, relPath)
	}
	return out, nil
}

func (s *testPersistStorage) MaterializeToDir(ctx context.Context, teamID string, dstDir string) error {
	return nil
}

func (s *testPersistStorage) PutTaskGlobal(ctx context.Context, ref blob.RunObjectRef, r io.Reader) error {
	return nil
}

func (s *testPersistStorage) GetTaskGlobal(ctx context.Context, ref blob.RunObjectRef) ([]byte, error) {
	return nil, blob.ErrNotFound
}

func (s *testPersistStorage) PutTaskRunArtifacts(ctx context.Context, ref blob.RunObjectRef, r io.Reader) error {
	return nil
}

func (s *testPersistStorage) GetTaskRunArtifacts(ctx context.Context, ref blob.RunObjectRef) ([]byte, error) {
	return nil, blob.ErrNotFound
}

func TestTeamScopedFilesHandlers(t *testing.T) {
	teamA := "tm_personal_u1"
	teamB := "tm_shared_u1"
	persist := newTestPersistStorage()
	persist.files[teamA] = map[string][]byte{"team-a.txt": []byte("alpha")}
	persist.files[teamB] = map[string][]byte{"team-b.txt": []byte("beta")}

	h := NewHandler(Config{
		JWTSecret:      filesTestSecret,
		TeamStore:      &testutil.MockTeamStore{Teams: []entity.Team{{TeamID: teamA, Name: "My Space", PersonalForUserID: testutil.PtrString("u1"), CreatedBy: "u1"}, {TeamID: teamB, Name: "Shared", CreatedBy: "u1"}}, Members: []entity.TeamMember{{TeamID: teamA, UserID: "u1", Role: entity.TeamRoleOwner}, {TeamID: teamB, UserID: "u1", Role: entity.TeamRoleOwner}}},
		PersistStorage: persist,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	token := testutil.SignJWT("u1", filesTestSecret)

	t.Run("list files uses team scope", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamA+"/files", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "team-a.txt") {
			t.Fatalf("body %q missing team-a.txt", body)
		}
		if strings.Contains(body, "team-b.txt") {
			t.Fatalf("body %q should not include team-b.txt", body)
		}
	})

	t.Run("file content uses team scope", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamB+"/files/team-b.txt", nil)
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

	t.Run("upload stores file in selected team", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("files", "notes.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("shared-team")); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+teamB+"/upload", body)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if string(persist.files[teamB]["notes.txt"]) != "shared-team" {
			t.Fatalf("teamB notes.txt = %q, want %q", persist.files[teamB]["notes.txt"], "shared-team")
		}
		if _, ok := persist.files[teamA]["notes.txt"]; ok {
			t.Fatal("teamA should not receive uploaded file")
		}
	})

	t.Run("non member cannot access team files", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/tm_other/files", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}
