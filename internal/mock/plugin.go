package mock

import (
	"bytes"
	"context"
	"io"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/objectstore"
)

// MockPluginStore is an in-memory model.PluginStore for tests.
type MockPluginStore struct {
	plugins  map[string]*model.Plugin
	releases map[string][]*model.PluginRelease
	nextID   int
}

func NewMockPluginStore() *MockPluginStore {
	return &MockPluginStore{plugins: map[string]*model.Plugin{}, releases: map[string][]*model.PluginRelease{}}
}

func (f *MockPluginStore) newID(prefix string) string {
	f.nextID++
	return prefix + string(rune('a'+f.nextID%26))
}

func (f *MockPluginStore) CreatePlugin(_ context.Context, in model.CreatePluginInput) (*model.Plugin, error) {
	if _, taken := f.plugins[in.Name]; taken {
		return nil, model.ErrPluginNameTaken
	}
	p := &model.Plugin{
		PluginID: f.newID("pl_"), Name: in.Name, DisplayName: in.DisplayName,
		Description: in.Description, CreatedBy: in.CreatedBy,
	}
	f.plugins[in.Name] = p
	return p, nil
}

func (f *MockPluginStore) GetPlugin(_ context.Context, name string) (*model.Plugin, error) {
	p, ok := f.plugins[name]
	if !ok {
		return nil, nil
	}
	copied := *p
	return &copied, nil
}

func (f *MockPluginStore) ListPlugins(_ context.Context, includeArchived bool) ([]model.Plugin, error) {
	var out []model.Plugin
	for _, p := range f.plugins {
		if !includeArchived && p.Archived() {
			continue
		}
		out = append(out, *p)
	}
	return out, nil
}

func (f *MockPluginStore) UpdatePlugin(_ context.Context, name string, in model.UpdatePluginInput) (*model.Plugin, error) {
	p, ok := f.plugins[name]
	if !ok {
		return nil, model.ErrNotFound
	}
	p.DisplayName, p.Description = in.DisplayName, in.Description
	copied := *p
	return &copied, nil
}

func (f *MockPluginStore) SetPluginArchived(_ context.Context, name string, archived bool) error {
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

func (f *MockPluginStore) CreatePluginRelease(_ context.Context, in model.CreatePluginReleaseInput) (*model.PluginRelease, error) {
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
		PluginReleaseID: f.newID("plr_"), PluginID: entry.PluginID, PluginName: in.PluginName,
		Version: in.Version, MinBuildmaxVersion: in.MinBuildmaxVersion, Digest: in.Digest,
		ObjectKey: in.ObjectKey, SizeBytes: in.SizeBytes, Inspection: in.Inspection,
		Source: in.Source, PublishedBy: in.PublishedBy,
	}
	f.releases[in.PluginName] = append(f.releases[in.PluginName], r)
	return r, nil
}

func (f *MockPluginStore) GetPluginRelease(_ context.Context, name, version string) (*model.PluginRelease, error) {
	for _, r := range f.releases[name] {
		if r.Version == version {
			copied := *r
			return &copied, nil
		}
	}
	return nil, nil
}

func (f *MockPluginStore) ListPluginReleases(_ context.Context, name string) ([]model.PluginRelease, error) {
	var out []model.PluginRelease
	for _, r := range f.releases[name] {
		out = append(out, *r)
	}
	return out, nil
}

func (f *MockPluginStore) YankPluginRelease(_ context.Context, name, version, actor, reason string) error {
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

// MockPluginPackageStorage is an in-memory objectstore.PluginPackageStorage.
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
		return nil, 0, objectstore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (f *MockPluginPackageStorage) Exists(_ context.Context, key string) (bool, error) {
	_, ok := f.Objects[key]
	return ok, nil
}
