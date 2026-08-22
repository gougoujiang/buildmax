package artifact

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

const (
	testSecret = "artifact-secret"
	teamA      = "tm_a"
	teamB      = "tm_b"
	userOwner  = "u_owner"
	userMember = "u_member"
	userOther  = "u_other"
	userStrand = "u_outsider"
)

type fixture struct {
	mux     *http.ServeMux
	store   *mock.MockArtifactStore
	storage *mock.MockArtifactStorage
	svc     *artifactsvc.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	store := &mock.MockArtifactStore{}
	storage := mock.NewMockArtifactStorage()
	svc := &artifactsvc.Service{Artifacts: store, Storage: storage}
	teams := &mock.MockTeamStore{
		Teams: []model.Team{
			{ID: teamA, Name: "A", CreatedBy: userOwner},
			{ID: teamB, Name: "B", CreatedBy: userOther},
		},
		Members: []model.TeamMember{
			{TeamID: teamA, UserID: userOwner, Role: model.TeamRoleOwner},
			{TeamID: teamA, UserID: userMember, Role: model.TeamRoleMember},
			// The stranger is a legitimate member of somewhere else, which is
			// the case that separates "is a member" from "is a member of this".
			{TeamID: teamB, UserID: userOther, Role: model.TeamRoleOwner},
		},
	}
	h := New(Config{
		JWTSecret: testSecret,
		Users:     &mock.MockUserStore{},
		Teams:     teams,
		Artifacts: svc,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return &fixture{mux: mux, store: store, storage: storage, svc: svc}
}

func (f *fixture) do(t *testing.T, method, path, userID string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, testSecret))
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func multipartBody(t *testing.T, filename, content string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(uploadFormField, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &buf, w.FormDataContentType()
}

func (f *fixture) upload(t *testing.T, userID, teamID, filename, content string) ArtifactResponse {
	t.Helper()
	body, contentType := multipartBody(t, filename, content)
	rec := f.do(t, http.MethodPost, "/api/teams/"+teamID+"/artifacts", userID, body, contentType)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var out ArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	return out
}

func TestUploadThenReadByID(t *testing.T) {
	f := newFixture(t)
	created := f.upload(t, userOwner, teamA, "report.md", "# hello")

	if _, ok := util.CanonicalPublicID(created.ID); !ok {
		t.Fatalf("artifact id = %q, want a canonical public ID", created.ID)
	}
	// A different member of the same team resolves the same reference, with no
	// team named anywhere in the URL.
	rec := f.do(t, http.MethodGet, "/api/artifacts/"+created.ID, userMember, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rec.Code)
	}
	content := f.do(t, http.MethodGet, "/api/artifacts/"+created.ID+"/content", userMember, nil, "")
	if content.Code != http.StatusOK {
		t.Fatalf("content status = %d, want 200", content.Code)
	}
	if content.Body.String() != "# hello" {
		t.Errorf("content = %q, want the uploaded bytes", content.Body.String())
	}
}

// The storage key is the one field that must never leave the server: it names
// deployment layout, and a client that learned it could come to depend on it.
func TestResponsesNeverCarryTheStorageKey(t *testing.T) {
	f := newFixture(t)
	created := f.upload(t, userOwner, teamA, "report.md", "# hello")

	stored, err := f.store.GetArtifact(t.Context(), created.ID)
	if err != nil || stored == nil {
		t.Fatalf("stored record: %v", err)
	}
	if stored.StorageKey == "" {
		t.Fatal("the record should have kept the key the storage reported")
	}
	for _, path := range []string{
		"/api/artifacts/" + created.ID,
		"/api/teams/" + teamA + "/artifacts",
	} {
		body := f.do(t, http.MethodGet, path, userOwner, nil, "").Body.String()
		if strings.Contains(body, stored.StorageKey) {
			t.Errorf("%s leaked the storage key", path)
		}
		if strings.Contains(body, "storage_key") {
			t.Errorf("%s serialized a storage_key field", path)
		}
	}
}

// An opaque ID is an identifier and not a credential. Answering 403 would make
// the route an oracle for which IDs exist, so a stranger gets exactly what they
// get for an ID that was never issued.
func TestNonMemberCannotTellAnArtifactFromNothing(t *testing.T) {
	f := newFixture(t)
	created := f.upload(t, userOwner, teamA, "report.md", "# hello")

	real := f.do(t, http.MethodGet, "/api/artifacts/"+created.ID, userOther, nil, "")
	invented := f.do(t, http.MethodGet, "/api/artifacts/ar_doesnotexist000000000", userOther, nil, "")

	if real.Code != http.StatusNotFound {
		t.Errorf("a non-member got %d for a real artifact, want 404", real.Code)
	}
	if invented.Code != http.StatusNotFound {
		t.Errorf("a non-member got %d for an invented id, want 404", invented.Code)
	}
	if real.Body.String() != invented.Body.String() {
		t.Errorf("the two answers differ, which tells a stranger the id exists:\n real: %s\n fake: %s",
			real.Body.String(), invented.Body.String())
	}
	if code := f.do(t, http.MethodGet, "/api/artifacts/"+created.ID+"/content", userOther, nil, "").Code; code != http.StatusNotFound {
		t.Errorf("content for a non-member = %d, want 404", code)
	}
}

func TestAnonymousCallerIsUnauthorized(t *testing.T) {
	f := newFixture(t)
	created := f.upload(t, userOwner, teamA, "report.md", "# hello")
	if code := f.do(t, http.MethodGet, "/api/artifacts/"+created.ID, "", nil, "").Code; code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
}

func TestListShowsOnlyTheTeamsOwnArtifacts(t *testing.T) {
	f := newFixture(t)
	f.upload(t, userOwner, teamA, "a.md", "a")
	f.upload(t, userOther, teamB, "b.md", "b")

	rec := f.do(t, http.MethodGet, "/api/teams/"+teamA+"/artifacts", userMember, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out artifactListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Total != 1 || len(out.Items) != 1 || out.Items[0].Filename != "a.md" {
		t.Errorf("listing = %+v, want only the team's own artifact", out)
	}
}

// Anything a browser might execute leaves as a download, announced as bytes
// rather than as the type it claims to be.
func TestContentDispositionFollowsTheInlineAllowlist(t *testing.T) {
	cases := []struct {
		filename    string
		wantInline  bool
		wantType    string
		wantDisposi string
	}{
		{"notes.txt", true, "text/plain; charset=utf-8", "inline"},
		{"logo.png", true, "image/png", "inline"},
		{"page.html", false, artifactsvc.FallbackMediaType, "attachment"},
		{"icon.svg", false, artifactsvc.FallbackMediaType, "attachment"},
		{"paper.pdf", false, artifactsvc.FallbackMediaType, "attachment"},
		{"archive.zip", false, artifactsvc.FallbackMediaType, "attachment"},
	}
	for _, c := range cases {
		t.Run(c.filename, func(t *testing.T) {
			f := newFixture(t)
			created := f.upload(t, userOwner, teamA, c.filename, "content")
			if created.Inline != c.wantInline {
				t.Errorf("inline = %v, want %v", created.Inline, c.wantInline)
			}
			rec := f.do(t, http.MethodGet, "/api/artifacts/"+created.ID+"/content", userOwner, nil, "")
			if got := rec.Header().Get("Content-Type"); got != c.wantType {
				t.Errorf("Content-Type = %q, want %q", got, c.wantType)
			}
			if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, c.wantDisposi) {
				t.Errorf("Content-Disposition = %q, want it to start with %q", got, c.wantDisposi)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
	}
}

func TestContentDispositionCarriesBothFilenameForms(t *testing.T) {
	f := newFixture(t)
	created := f.upload(t, userOwner, teamA, "报告 v2.txt", "content")
	rec := f.do(t, http.MethodGet, "/api/artifacts/"+created.ID+"/content", userOwner, nil, "")
	got := rec.Header().Get("Content-Disposition")
	if !strings.Contains(got, `filename="`) || !strings.Contains(got, "filename*=UTF-8''") {
		t.Errorf("Content-Disposition = %q, want an ASCII fallback and an RFC 5987 form", got)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("Content-Disposition = %q, must not carry a line break", got)
	}
}

// A member may withdraw what they put there; a colleague's file and a run's
// output are not theirs to withdraw.
func TestDeletePolicy(t *testing.T) {
	f := newFixture(t)
	mine := f.upload(t, userMember, teamA, "mine.md", "mine")
	theirs := f.upload(t, userOwner, teamA, "theirs.md", "theirs")

	if code := f.do(t, http.MethodDelete, "/api/artifacts/"+theirs.ID, userMember, nil, "").Code; code != http.StatusForbidden {
		t.Errorf("a member deleting someone else's artifact got %d, want 403", code)
	}
	if code := f.do(t, http.MethodDelete, "/api/artifacts/"+mine.ID, userMember, nil, "").Code; code != http.StatusNoContent {
		t.Errorf("a member deleting their own artifact got %d, want 204", code)
	}
	if code := f.do(t, http.MethodGet, "/api/artifacts/"+mine.ID, userMember, nil, "").Code; code != http.StatusNotFound {
		t.Errorf("a deleted artifact is still readable: %d", code)
	}
	if code := f.do(t, http.MethodDelete, "/api/artifacts/"+theirs.ID, userOwner, nil, "").Code; code != http.StatusNoContent {
		t.Errorf("an owner deleting any team artifact got %d, want 204", code)
	}
}

func TestDeleteByNonMemberIsNotFound(t *testing.T) {
	f := newFixture(t)
	created := f.upload(t, userOwner, teamA, "report.md", "hello")
	if code := f.do(t, http.MethodDelete, "/api/artifacts/"+created.ID, userOther, nil, "").Code; code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

func TestUploadWithoutAFilePart(t *testing.T) {
	f := newFixture(t)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("title", "no file here"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	rec := f.do(t, http.MethodPost, "/api/teams/"+teamA+"/artifacts", userOwner, &buf, w.FormDataContentType())
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestUploadOverTheLimitIsRefused(t *testing.T) {
	f := newFixture(t)
	f.svc.MaxFileBytes = 4
	body, contentType := multipartBody(t, "big.bin", strings.Repeat("a", 64))
	rec := f.do(t, http.MethodPost, "/api/teams/"+teamA+"/artifacts", userOwner, body, contentType)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %s", rec.Code, rec.Body.String())
	}
	if f.store.Count() != 0 || f.storage.ObjectCount() != 0 {
		t.Error("a refused upload must leave neither a record nor an object")
	}
}

func TestUploadRecordsProvenance(t *testing.T) {
	f := newFixture(t)
	created := f.upload(t, userOwner, teamA, "report.md", "hello")
	if created.SourceType != model.ArtifactSourceUserUpload {
		t.Errorf("source type = %q, want %q", created.SourceType, model.ArtifactSourceUserUpload)
	}
	if created.CreatedByType != model.ArtifactCreatorUser || created.CreatedByID != userOwner {
		t.Errorf("creator = %q/%q, want a user and the uploader", created.CreatedByType, created.CreatedByID)
	}
}

func TestUploadTitleComesFromTheQuery(t *testing.T) {
	f := newFixture(t)
	body, contentType := multipartBody(t, "report.md", "hello")
	rec := f.do(t, http.MethodPost, "/api/teams/"+teamA+"/artifacts?title=Quarterly+report", userOwner, body, contentType)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	var out ArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Title != "Quarterly report" {
		t.Errorf("title = %q, want the query value", out.Title)
	}
}

// Every route must refuse before it says anything about the deployment, so an
// unconfigured server does not answer an anonymous caller at all.
func TestUnconfiguredDeploymentStillAuthenticatesFirst(t *testing.T) {
	h := New(Config{
		JWTSecret: testSecret,
		Users:     &mock.MockUserStore{},
		Teams:     &mock.MockTeamStore{},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	for _, path := range []string{"/api/artifacts/ar_x", "/api/artifacts/ar_x/content", "/api/teams/" + teamA + "/artifacts"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s answered an anonymous caller with %d, want 401", path, rec.Code)
		}
	}
}

// A client with a login but no chosen team gets its personal one. This is what
// lets CLI and Desktop publish without ever being told about teams.
func TestUploadToDefaultTeamUsesThePersonalTeam(t *testing.T) {
	store := &mock.MockArtifactStore{}
	storage := mock.NewMockArtifactStorage()
	personal := "tm_personal"
	teams := &mock.MockTeamStore{
		Teams: []model.Team{
			{ID: personal, Name: "Mine", PersonalForUserID: util.Ptr(userOwner), CreatedBy: userOwner},
			{ID: teamA, Name: "A", CreatedBy: userOwner},
		},
		Members: []model.TeamMember{
			{TeamID: personal, UserID: userOwner, Role: model.TeamRoleOwner},
			{TeamID: teamA, UserID: userOwner, Role: model.TeamRoleMember},
		},
	}
	h := New(Config{
		JWTSecret: testSecret,
		Users:     &mock.MockUserStore{},
		Teams:     teams,
		Artifacts: &artifactsvc.Service{Artifacts: store, Storage: storage},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	f := &fixture{mux: mux, store: store, storage: storage}

	body, contentType := multipartBody(t, "report.md", "hello")
	rec := f.do(t, http.MethodPost, "/api/artifacts", userOwner, body, contentType)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var out ArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.TeamID != personal {
		t.Errorf("team = %q, want the caller's personal team %q", out.TeamID, personal)
	}

	// An explicit team is honoured, and still checked for membership.
	body, contentType = multipartBody(t, "other.md", "hello")
	rec = f.do(t, http.MethodPost, "/api/artifacts?team_id="+teamA, userOwner, body, contentType)
	if rec.Code != http.StatusCreated {
		t.Fatalf("explicit team status = %d, want 201", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.TeamID != teamA {
		t.Errorf("team = %q, want %q", out.TeamID, teamA)
	}
}

func TestUploadToDefaultTeamRefusesATeamTheCallerIsNotIn(t *testing.T) {
	f := newFixture(t)
	body, contentType := multipartBody(t, "report.md", "hello")
	rec := f.do(t, http.MethodPost, "/api/artifacts?team_id="+teamB, userOwner, body, contentType)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if f.store.Count() != 0 {
		t.Error("a refused upload must not be recorded")
	}
}
