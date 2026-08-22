package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/core/plugin/archive"
	"github.com/gougoujiang/buildmax/internal/infra/pluginwire"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

const (
	catalogSecret = "catalog-secret"
	catalogUser   = "u_reader"
)

type catalogFixture struct {
	mux      *http.ServeMux
	service  *pluginsvc.Service
	packages *mock.MockPluginPackageStorage
}

func newCatalogFixture(t *testing.T, withMarketplace bool) *catalogFixture {
	t.Helper()
	users := &mock.MockUserStore{
		ByID:    map[string]*model.User{catalogUser: {ID: catalogUser, Email: "reader@example.com", CreatedAt: 1}},
		ByEmail: map[string]*model.User{"reader@example.com": {ID: catalogUser, Email: "reader@example.com", CreatedAt: 1}},
	}
	audits := &mock.MockAuditStore{}
	cfg := Config{
		JWTSecret:  catalogSecret,
		UserStore:  users,
		TeamStore:  &mock.MockTeamStore{},
		AuditStore: audits,
		Audit:      audit.NewRecorder(audits),
	}
	f := &catalogFixture{packages: mock.NewMockPluginPackageStorage()}
	if withMarketplace {
		f.service = &pluginsvc.Service{
			Catalog:  mock.NewMockPluginStore(),
			Packages: f.packages,
			Audit:    audit.NewRecorder(audits),
		}
		cfg.PluginService = f.service
	}
	h := NewHandler(cfg)
	f.mux = http.NewServeMux()
	h.Register(f.mux)
	return f
}

func (f *catalogFixture) get(t *testing.T, path, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, catalogSecret))
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func (f *catalogFixture) publish(t *testing.T, manifest string) *model.PluginRelease {
	t.Helper()
	var buf bytes.Buffer
	if _, err := archive.Pack(&buf, fstest.MapFS{
		"plugin.yaml":            &fstest.MapFile{Data: []byte(manifest)},
		"skills/review/SKILL.md": &fstest.MapFile{Data: []byte("# review\n")},
	}, archive.Limits{}); err != nil {
		t.Fatal(err)
	}
	name := manifestName(t, manifest)
	release, err := f.service.Publish(context.Background(), pluginsvc.PublishInput{
		PluginName: name, Body: bytes.NewReader(buf.Bytes()), ActorID: "u_admin",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return release
}

func manifestName(t *testing.T, manifest string) string {
	t.Helper()
	for _, line := range strings.Split(manifest, "\n") {
		if rest, ok := strings.CutPrefix(line, "name: "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatalf("manifest has no name: %q", manifest)
	return ""
}

// Browsing changes nothing, so any active account may do it — and no account
// at all may not.
func TestCatalogRequiresAnAccount(t *testing.T) {
	f := newCatalogFixture(t, true)
	for _, path := range []string{
		"/api/plugins",
		"/api/plugins/code-review",
		"/api/plugins/code-review/releases/1.0.0/download",
	} {
		if got := f.get(t, path, "").Code; got != http.StatusUnauthorized {
			t.Errorf("GET %s anonymous = %d, want 401", path, got)
		}
	}
}

func TestCatalogListAndDetail(t *testing.T) {
	f := newCatalogFixture(t, true)
	f.publish(t, "name: code-review\nversion: 1.0.0\ndescription: Reviews.\n")
	f.publish(t, "name: code-review\nversion: 1.1.0\n")

	rec := f.get(t, "/api/plugins", catalogUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body)
	}
	var list pluginwire.CatalogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Plugins) != 1 || list.Plugins[0].Name != "code-review" {
		t.Fatalf("catalog = %+v", list.Plugins)
	}

	rec = f.get(t, "/api/plugins/code-review", catalogUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail = %d: %s", rec.Code, rec.Body)
	}
	var detail pluginwire.PluginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Releases) != 2 {
		t.Errorf("releases = %+v", detail.Releases)
	}
	// A record that would let somebody install this must not carry the inside
	// of the publisher's configuration.
	if detail.Releases[0].Digest == "" || detail.Releases[0].ObjectKey == "" {
		t.Errorf("release = %+v", detail.Releases[0])
	}

	if got := f.get(t, "/api/plugins/absent", catalogUser).Code; got != http.StatusNotFound {
		t.Errorf("missing plugin = %d, want 404", got)
	}
}

// Archiving takes an entry out of the default catalog without deleting it, so
// a saved link keeps working.
func TestCatalogHidesArchivedFromTheListing(t *testing.T) {
	f := newCatalogFixture(t, true)
	f.publish(t, "name: code-review\nversion: 1.0.0\n")
	if err := f.service.SetArchived(context.Background(), "code-review", true, "u_admin"); err != nil {
		t.Fatal(err)
	}

	rec := f.get(t, "/api/plugins", catalogUser)
	var list pluginwire.CatalogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Plugins) != 0 {
		t.Errorf("an archived entry is still listed: %+v", list.Plugins)
	}
	if got := f.get(t, "/api/plugins/code-review", catalogUser).Code; got != http.StatusOK {
		t.Errorf("direct link = %d, want 200", got)
	}
	if got := f.get(t, "/api/plugins/code-review/releases/1.0.0/download", catalogUser).Code; got != http.StatusOK {
		t.Errorf("download from an archived entry = %d, want 200", got)
	}
}

func TestCatalogDownloadStreamsTheBytes(t *testing.T) {
	f := newCatalogFixture(t, true)
	release := f.publish(t, "name: code-review\nversion: 1.2.0\n")

	rec := f.get(t, "/api/plugins/code-review/releases/1.2.0/download", catalogUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("download = %d: %s", rec.Code, rec.Body)
	}
	// The digest travels with the bytes, so a client verifies what it received
	// without a second request.
	if got := rec.Header().Get("X-Buildmax-Digest"); got != release.Digest {
		t.Errorf("digest header = %q, want %q", got, release.Digest)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/gzip" {
		t.Errorf("content type = %q", got)
	}
	body := rec.Body.Bytes()
	if int64(len(body)) != release.SizeBytes {
		t.Errorf("read %d bytes, want %d", len(body), release.SizeBytes)
	}
	if err := archive.VerifyDigest(bytes.NewReader(body), release.Digest); err != nil {
		t.Errorf("what was served does not match what was published: %v", err)
	}

	if got := f.get(t, "/api/plugins/code-review/releases/9.9.9/download", catalogUser).Code; got != http.StatusNotFound {
		t.Errorf("missing version = %d, want 404", got)
	}
}

// A withdrawn release is still downloadable, but not by accident.
func TestCatalogDownloadOfAWithdrawnRelease(t *testing.T) {
	f := newCatalogFixture(t, true)
	f.publish(t, "name: code-review\nversion: 1.0.0\n")
	if err := f.service.Yank(context.Background(), "code-review", "1.0.0", "u_admin", "broken hook"); err != nil {
		t.Fatal(err)
	}

	rec := f.get(t, "/api/plugins/code-review/releases/1.0.0/download", catalogUser)
	if rec.Code != http.StatusConflict {
		t.Fatalf("download = %d, want 409: %s", rec.Code, rec.Body)
	}
	// The refusal says why and how to proceed, because a recovery needs both.
	if !strings.Contains(rec.Body.String(), "broken hook") || !strings.Contains(rec.Body.String(), "allow_yanked") {
		t.Errorf("body = %s", rec.Body)
	}
	rec = f.get(t, "/api/plugins/code-review/releases/1.0.0/download?allow_yanked=true", catalogUser)
	if rec.Code != http.StatusOK {
		t.Errorf("acknowledged download = %d: %s", rec.Code, rec.Body)
	}
}

// A release whose bytes went missing is a 404 about the bytes, not a 500: the
// catalog is intact and says so.
func TestCatalogDownloadWithMissingBytes(t *testing.T) {
	f := newCatalogFixture(t, true)
	release := f.publish(t, "name: code-review\nversion: 1.0.0\n")
	delete(f.packages.Objects, release.ObjectKey)

	if got := f.get(t, "/api/plugins/code-review/releases/1.0.0/download", catalogUser).Code; got != http.StatusNotFound {
		t.Errorf("download = %d, want 404", got)
	}
}

func TestCatalogWithoutAMarketplace(t *testing.T) {
	f := newCatalogFixture(t, false)
	for _, path := range []string{"/api/plugins", "/api/plugins/code-review"} {
		if got := f.get(t, path, catalogUser).Code; got != http.StatusServiceUnavailable {
			t.Errorf("GET %s = %d, want 503", path, got)
		}
	}
}
