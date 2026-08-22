package plugin

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/core/plugin/archive"
	"github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/service/audit"
)

func file(body string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(body)} }

// validPackage is the smallest thing publication accepts.
func validPackage(t *testing.T, manifest string, extra fstest.MapFS) []byte {
	t.Helper()
	src := fstest.MapFS{"plugin.yaml": file(manifest)}
	for name, f := range extra {
		src[name] = f
	}
	var buf bytes.Buffer
	if _, err := archive.Pack(&buf, src, archive.Limits{}); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	return buf.Bytes()
}

func newService(t *testing.T) (*Service, *fakeCatalog, *fakePackages, *fakeAudit) {
	t.Helper()
	catalog := newFakeCatalog()
	packages := &fakePackages{objects: map[string][]byte{}}
	events := &fakeAudit{}
	return &Service{
		Catalog:   catalog,
		Packages:  packages,
		KeyPrefix: "bm",
		Audit:     audit.NewRecorder(events),
	}, catalog, packages, events
}

func TestPublishStoresBytesAndRelease(t *testing.T) {
	s, catalog, packages, events := newService(t)
	ctx := context.Background()
	data := validPackage(t, "name: code-review\nversion: 1.2.0\ndescription: Reviews.\n"+
		"display_name: Code Review\nmin_buildmax_version: 0.9.0\n", fstest.MapFS{
		"skills/review/SKILL.md": file("# review\n"),
	})

	release, err := s.Publish(ctx, PublishInput{
		PluginName: "code-review",
		Body:       bytes.NewReader(data),
		Source:     model.PluginReleaseSource{RemoteURL: "git@example.com:x.git", Commit: "abc123"},
		ActorID:    "u_admin",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if release.Version != "1.2.0" || release.MinBuildmaxVersion != "0.9.0" {
		t.Errorf("release = %+v", release)
	}
	if release.SizeBytes != int64(len(data)) {
		t.Errorf("SizeBytes = %d, want %d", release.SizeBytes, len(data))
	}
	// The digest is what the server hashed, not what the publisher said.
	wantDigest, err := archive.Digest(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if release.Digest != wantDigest {
		t.Errorf("Digest = %q, want %q", release.Digest, wantDigest)
	}
	if len(release.Inspection.Skills) != 1 || release.Inspection.Skills[0] != "review" {
		t.Errorf("inspection = %+v", release.Inspection)
	}
	if release.Source.Commit != "abc123" {
		t.Errorf("source claim not recorded: %+v", release.Source)
	}

	stored, ok := packages.objects[release.ObjectKey]
	if !ok {
		t.Fatalf("no bytes at %q", release.ObjectKey)
	}
	if !bytes.Equal(stored, data) {
		t.Error("stored bytes differ from what was uploaded")
	}

	// A first publish creates the entry, and the trail says so.
	entry, _ := catalog.GetPlugin(ctx, "code-review")
	if entry == nil || entry.DisplayName != "Code Review" {
		t.Errorf("entry = %+v", entry)
	}
	if !events.has(model.AuditPluginCreated) || !events.has(model.AuditPluginPublished) {
		t.Errorf("audit trail = %+v", events.actions())
	}
	// The trail names the version and a digest prefix, never the whole thing.
	detail := events.detailFor(model.AuditPluginPublished)
	if !strings.Contains(detail, "code-review@1.2.0") {
		t.Errorf("detail = %q", detail)
	}
	if strings.Contains(detail, strings.TrimPrefix(wantDigest, archive.DigestPrefix)) {
		t.Errorf("detail carries the full digest: %q", detail)
	}
}

// A release is what somebody reviewed and what somebody else downloaded, so a
// second publication of one version is refused even for identical bytes.
func TestPublishRefusesAnExistingVersion(t *testing.T) {
	s, _, _, _ := newService(t)
	ctx := context.Background()
	data := validPackage(t, "name: code-review\nversion: 1.2.0\n", nil)

	if _, err := s.Publish(ctx, PublishInput{PluginName: "code-review", Body: bytes.NewReader(data), ActorID: "u_admin"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Publish(ctx, PublishInput{PluginName: "code-review", Body: bytes.NewReader(data), ActorID: "u_admin"})
	if !errors.Is(err, model.ErrPluginVersionExists) {
		t.Errorf("err = %v, want ErrPluginVersionExists", err)
	}
}

func TestPublishRefusals(t *testing.T) {
	tests := []struct {
		name     string
		route    string
		manifest string
		extra    fstest.MapFS
		want     error
	}{
		{
			name: "manifest names another plugin", route: "code-review",
			manifest: "name: something-else\nversion: 1.0.0\n", want: ErrNameMismatch,
		},
		{
			name: "no version", route: "code-review",
			manifest: "name: code-review\n", want: ErrInvalidPackage,
		},
		{
			name: "unparseable version", route: "code-review",
			manifest: "name: code-review\nversion: v1\n", want: ErrInvalidPackage,
		},
		{
			name: "unreadable bound", route: "code-review",
			manifest: "name: code-review\nversion: 1.0.0\nmin_buildmax_version: \">=0.9\"\n", want: ErrInvalidPackage,
		},
		{
			name: "payload that will not load", route: "code-review",
			manifest: "name: code-review\nversion: 1.0.0\n",
			extra:    fstest.MapFS{"mcp.json": file("{not json")}, want: ErrInvalidPackage,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _, packages, _ := newService(t)
			data := validPackage(t, tc.manifest, tc.extra)
			_, err := s.Publish(context.Background(), PublishInput{
				PluginName: tc.route, Body: bytes.NewReader(data), ActorID: "u_admin",
			})
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			// A refused publish stores nothing.
			if len(packages.objects) != 0 {
				t.Errorf("refused publish stored %d objects", len(packages.objects))
			}
		})
	}
}

// The server hashes, extracts, and inspects what it received rather than
// trusting the client that sent it.
func TestPublishRefusesSomethingThatIsNotAnArchive(t *testing.T) {
	s, _, _, _ := newService(t)
	_, err := s.Publish(context.Background(), PublishInput{
		PluginName: "code-review", Body: strings.NewReader("not an archive"), ActorID: "u_admin",
	})
	if !errors.Is(err, ErrInvalidPackage) {
		t.Errorf("err = %v, want ErrInvalidPackage", err)
	}
}

func TestPublishStopsAnOversizedUpload(t *testing.T) {
	s, _, packages, _ := newService(t)
	s.Limits = archive.Limits{MaxCompressedBytes: 64}
	data := validPackage(t, "name: code-review\nversion: 1.0.0\n", fstest.MapFS{
		"skills/big/SKILL.md": file(strings.Repeat("x", 8192)),
	})
	_, err := s.Publish(context.Background(), PublishInput{
		PluginName: "code-review", Body: bytes.NewReader(data), ActorID: "u_admin",
	})
	if !errors.Is(err, ErrInvalidPackage) {
		t.Fatalf("err = %v, want ErrInvalidPackage", err)
	}
	if len(packages.objects) != 0 {
		t.Error("an oversized upload reached storage")
	}
}

func TestPublishRefusesAnArchivedEntry(t *testing.T) {
	s, _, _, _ := newService(t)
	ctx := context.Background()
	if _, err := s.CreateEntry(ctx, CreateEntryInput{Name: "code-review", ActorID: "u_admin"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetArchived(ctx, "code-review", true, "u_admin"); err != nil {
		t.Fatal(err)
	}
	data := validPackage(t, "name: code-review\nversion: 1.0.0\n", nil)
	_, err := s.Publish(ctx, PublishInput{PluginName: "code-review", Body: bytes.NewReader(data), ActorID: "u_admin"})
	if !errors.Is(err, model.ErrPluginArchived) {
		t.Errorf("err = %v, want ErrPluginArchived", err)
	}
}

func TestCatalogLifecycleRecordsTheTrail(t *testing.T) {
	s, _, _, events := newService(t)
	ctx := context.Background()

	if _, err := s.CreateEntry(ctx, CreateEntryInput{Name: "code-review", ActorID: "u_admin"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEntry(ctx, CreateEntryInput{Name: "Bad Name", ActorID: "u_admin"}); !errors.Is(err, ErrInvalidPackage) {
		t.Errorf("an invalid name: err = %v, want ErrInvalidPackage", err)
	}
	if _, err := s.UpdateEntry(ctx, "code-review", model.UpdatePluginInput{Description: "New."}, "u_admin"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetArchived(ctx, "code-review", true, "u_admin"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetArchived(ctx, "code-review", false, "u_admin"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetArchived(ctx, "absent", true, "u_admin"); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("archiving a missing entry: err = %v, want ErrNotFound", err)
	}
	for _, action := range []string{
		model.AuditPluginCreated, model.AuditPluginUpdated,
		model.AuditPluginArchived, model.AuditPluginUnarchived,
	} {
		if !events.has(action) {
			t.Errorf("missing %s in %v", action, events.actions())
		}
	}
}

func TestYank(t *testing.T) {
	s, _, _, events := newService(t)
	ctx := context.Background()
	data := validPackage(t, "name: code-review\nversion: 1.0.0\n", nil)
	if _, err := s.Publish(ctx, PublishInput{PluginName: "code-review", Body: bytes.NewReader(data), ActorID: "u_admin"}); err != nil {
		t.Fatal(err)
	}

	if err := s.Yank(ctx, "code-review", "1.0.0", "u_admin", "broken hook"); err != nil {
		t.Fatal(err)
	}
	if !events.has(model.AuditPluginYanked) {
		t.Errorf("audit trail = %v", events.actions())
	}
	if err := s.Yank(ctx, "code-review", "9.9.9", "u_admin", ""); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("yanking a missing release: err = %v, want ErrNotFound", err)
	}
}

// --- fakes -----------------------------------------------------------------

type fakeCatalog struct {
	plugins  map[string]*model.Plugin
	releases map[string][]*model.PluginRelease
	nextID   int
}

func newFakeCatalog() *fakeCatalog {
	return &fakeCatalog{plugins: map[string]*model.Plugin{}, releases: map[string][]*model.PluginRelease{}}
}

func (f *fakeCatalog) id(prefix string) string {
	f.nextID++
	return prefix + string(rune('a'+f.nextID%26))
}

func (f *fakeCatalog) CreatePlugin(_ context.Context, in model.CreatePluginInput) (*model.Plugin, error) {
	if _, taken := f.plugins[in.Name]; taken {
		return nil, model.ErrPluginNameTaken
	}
	p := &model.Plugin{
		PluginID: f.id("pl_"), Name: in.Name, DisplayName: in.DisplayName,
		Description: in.Description, CreatedBy: in.CreatedBy,
	}
	f.plugins[in.Name] = p
	return p, nil
}

func (f *fakeCatalog) GetPlugin(_ context.Context, name string) (*model.Plugin, error) {
	p, ok := f.plugins[name]
	if !ok {
		return nil, nil
	}
	copied := *p
	return &copied, nil
}

func (f *fakeCatalog) ListPlugins(_ context.Context, includeArchived bool) ([]model.Plugin, error) {
	var out []model.Plugin
	for _, p := range f.plugins {
		if !includeArchived && p.Archived() {
			continue
		}
		out = append(out, *p)
	}
	return out, nil
}

func (f *fakeCatalog) UpdatePlugin(_ context.Context, name string, in model.UpdatePluginInput) (*model.Plugin, error) {
	p, ok := f.plugins[name]
	if !ok {
		return nil, model.ErrNotFound
	}
	p.DisplayName, p.Description = in.DisplayName, in.Description
	copied := *p
	return &copied, nil
}

func (f *fakeCatalog) SetPluginArchived(_ context.Context, name string, archived bool) error {
	p, ok := f.plugins[name]
	if !ok {
		return model.ErrNotFound
	}
	if archived {
		p.ArchivedAt = 1
	} else {
		p.ArchivedAt = 0
	}
	return nil
}

func (f *fakeCatalog) CreatePluginRelease(_ context.Context, in model.CreatePluginReleaseInput) (*model.PluginRelease, error) {
	entry, ok := f.plugins[in.PluginName]
	if !ok {
		return nil, model.ErrNotFound
	}
	if entry.Archived() {
		return nil, model.ErrPluginArchived
	}
	for _, r := range f.releases[in.PluginName] {
		if r.Version == in.Version {
			return nil, model.ErrPluginVersionExists
		}
	}
	r := &model.PluginRelease{
		PluginReleaseID: f.id("plr_"), PluginID: entry.PluginID, PluginName: in.PluginName,
		Version: in.Version, MinBuildmaxVersion: in.MinBuildmaxVersion, Digest: in.Digest,
		ObjectKey: in.ObjectKey, SizeBytes: in.SizeBytes, Inspection: in.Inspection,
		Source: in.Source, PublishedBy: in.PublishedBy,
	}
	f.releases[in.PluginName] = append(f.releases[in.PluginName], r)
	return r, nil
}

func (f *fakeCatalog) GetPluginRelease(_ context.Context, name, version string) (*model.PluginRelease, error) {
	for _, r := range f.releases[name] {
		if r.Version == version {
			copied := *r
			return &copied, nil
		}
	}
	return nil, nil
}

func (f *fakeCatalog) ListPluginReleases(_ context.Context, name string) ([]model.PluginRelease, error) {
	var out []model.PluginRelease
	for _, r := range f.releases[name] {
		out = append(out, *r)
	}
	return out, nil
}

func (f *fakeCatalog) YankPluginRelease(_ context.Context, name, version, actor, reason string) error {
	for _, r := range f.releases[name] {
		if r.Version == version {
			if r.YankedAt == 0 {
				r.YankedAt, r.YankedBy, r.YankedReason = 1, actor, reason
			}
			return nil
		}
	}
	return model.ErrNotFound
}

type fakePackages struct{ objects map[string][]byte }

func (f *fakePackages) Put(_ context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objects[key] = data
	return nil
}

func (f *fakePackages) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	data, ok := f.objects[key]
	if !ok {
		return nil, 0, objectstore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (f *fakePackages) Exists(_ context.Context, key string) (bool, error) {
	_, ok := f.objects[key]
	return ok, nil
}

type fakeAudit struct{ events []model.AuditEvent }

func (f *fakeAudit) RecordAuditEvent(_ context.Context, e model.AuditEvent) error {
	f.events = append(f.events, e)
	return nil
}

func (f *fakeAudit) has(action string) bool {
	for _, e := range f.events {
		if e.Action == action {
			return true
		}
	}
	return false
}

func (f *fakeAudit) actions() []string {
	var out []string
	for _, e := range f.events {
		out = append(out, e.Action)
	}
	return out
}

func (f *fakeAudit) detailFor(action string) string {
	for _, e := range f.events {
		if e.Action == action {
			return e.Detail
		}
	}
	return ""
}
