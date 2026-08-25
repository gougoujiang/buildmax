package mock

import (
	"bytes"
	"context"
	"io"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/infra/objectstore"
)

// MockPluginStore is an in-memory Marketplace catalog for tests.
type MockPluginStore struct {
	plugins  map[string]*coreplugin.Plugin
	releases map[string][]*coreplugin.Release
}

func NewMockPluginStore() *MockPluginStore {
	return &MockPluginStore{plugins: map[string]*coreplugin.Plugin{}, releases: map[string][]*coreplugin.Release{}}
}

func (f *MockPluginStore) CreatePlugin(_ context.Context, in coreplugin.CreateInput) (*coreplugin.Plugin, error) {
	if _, taken := f.plugins[in.Name]; taken {
		return nil, coreplugin.ErrNameTaken
	}
	p := &coreplugin.Plugin{
		Name: in.Name, DisplayName: in.DisplayName,
		Description: in.Description, CreatedBy: in.CreatedBy,
	}
	f.plugins[in.Name] = p
	return p, nil
}

func (f *MockPluginStore) GetPlugin(_ context.Context, name string) (*coreplugin.Plugin, error) {
	p, ok := f.plugins[name]
	if !ok {
		return nil, nil
	}
	copied := *p
	return &copied, nil
}

func (f *MockPluginStore) ListPlugins(_ context.Context, includeArchived bool) ([]coreplugin.Plugin, error) {
	var out []coreplugin.Plugin
	for _, p := range f.plugins {
		if !includeArchived && p.Archived() {
			continue
		}
		out = append(out, *p)
	}
	return out, nil
}

func (f *MockPluginStore) UpdatePlugin(_ context.Context, name string, in coreplugin.UpdateInput) (*coreplugin.Plugin, error) {
	p, ok := f.plugins[name]
	if !ok {
		return nil, apierr.ErrNotFound
	}
	p.DisplayName, p.Description = in.DisplayName, in.Description
	copied := *p
	return &copied, nil
}

func (f *MockPluginStore) SetPluginArchived(_ context.Context, name string, archived bool) error {
	p, ok := f.plugins[name]
	if !ok {
		return apierr.ErrNotFound
	}
	if archived {
		at := seqTime(1)
		p.ArchivedAt = &at
	} else {
		p.ArchivedAt = nil
	}
	return nil
}

func (f *MockPluginStore) CreatePluginRelease(_ context.Context, in coreplugin.CreateReleaseInput) (*coreplugin.Release, error) {
	entry, ok := f.plugins[in.PluginName]
	if !ok {
		return nil, apierr.ErrNotFound
	}
	if entry.Archived() {
		return nil, coreplugin.ErrArchived
	}
	for _, r := range f.releases[in.PluginName] {
		if r.Version == in.Version {
			return nil, coreplugin.ErrVersionExists
		}
	}
	r := &coreplugin.Release{
		PluginName: in.PluginName,
		Version:    in.Version, MinBuildmaxVersion: in.MinBuildmaxVersion, Digest: in.Digest,
		ObjectKey: in.ObjectKey, SizeBytes: in.SizeBytes, Inspection: in.Inspection,
		Source: in.Source, PublishedBy: in.PublishedBy,
	}
	f.releases[in.PluginName] = append(f.releases[in.PluginName], r)
	return r, nil
}

func (f *MockPluginStore) GetPluginRelease(_ context.Context, name, version string) (*coreplugin.Release, error) {
	for _, r := range f.releases[name] {
		if r.Version == version {
			copied := *r
			return &copied, nil
		}
	}
	return nil, nil
}

func (f *MockPluginStore) ListPluginReleases(_ context.Context, name string) ([]coreplugin.Release, error) {
	var out []coreplugin.Release
	for _, r := range f.releases[name] {
		out = append(out, *r)
	}
	return out, nil
}

func (f *MockPluginStore) YankPluginRelease(_ context.Context, name, version, actor, reason string) error {
	for _, r := range f.releases[name] {
		if r.Version == version {
			if r.YankedAt == nil {
				at := seqTime(1)
				r.YankedAt, r.YankedBy, r.YankedReason = &at, actor, reason
			}
			return nil
		}
	}
	return apierr.ErrNotFound
}

// MockPluginPackageStorage is an in-memory plugin.PackageStore.
type MockPluginPackageStorage struct{ Objects map[string][]byte }

// NewMockPluginPackageStorage returns empty storage.
func NewMockPluginPackageStorage() *MockPluginPackageStorage {
	return &MockPluginPackageStorage{Objects: map[string][]byte{}}
}

func (f *MockPluginPackageStorage) Put(_ context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.Objects[key] = data
	return nil
}

func (f *MockPluginPackageStorage) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	data, ok := f.Objects[key]
	if !ok {
		return nil, 0, apierr.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

// PackageKey delegates to the real layout, so a key a test stores under is the
// key production would have used.
func (f *MockPluginPackageStorage) PackageKey(prefix, pluginName, digest string) (string, error) {
	return objectstore.PluginPackageKey(prefix, pluginName, digest)
}

func (f *MockPluginPackageStorage) Exists(_ context.Context, key string) (bool, error) {
	_, ok := f.Objects[key]
	return ok, nil
}
