package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// pluginRow is one catalog entry. It has no team column: the catalog belongs to
// the deployment, which is what lets a System Administrator manage company
// capabilities without reaching into any team's content.
type pluginRow struct {
	ID          uint   `gorm:"primaryKey;autoIncrement"`
	Name        string `gorm:"type:varchar(128);uniqueIndex;not null"`
	DisplayName string `gorm:"type:varchar(255);not null;default:''"`
	Description string `gorm:"type:varchar(1024);not null;default:''"`
	ArchivedAt  int64  `gorm:"not null;default:0;index"`
	CreatedBy   string `gorm:"type:varchar(64);not null"`
	CreatedAt   int64  `gorm:"autoCreateTime;index"`
	UpdatedAt   int64  `gorm:"autoUpdateTime"`
}

func (pluginRow) TableName() string { return "plugin" }

// pluginReleaseRow is one published version.
//
// Inspection and Source are JSON documents rather than columns because nothing
// queries inside them: they are written once and read whole, and giving each
// field a column would freeze the report's shape into the schema.
type pluginReleaseRow struct {
	ID uint `gorm:"primaryKey;autoIncrement"`
	// PluginName is denormalised so a release reads without a join. The unique
	// index over (plugin_name, version) is what makes a version immutable.
	PluginName         string `gorm:"type:varchar(128);not null;uniqueIndex:ux_plugin_release_version,priority:1"`
	Version            string `gorm:"type:varchar(64);not null;uniqueIndex:ux_plugin_release_version,priority:2"`
	MinBuildmaxVersion string `gorm:"type:varchar(64);not null;default:''"`

	Digest    string `gorm:"type:varchar(128);not null;index"`
	ObjectKey string `gorm:"type:varchar(512);not null"`
	SizeBytes int64  `gorm:"not null;default:0"`

	Inspection string `gorm:"type:text"`
	Source     string `gorm:"type:text"`

	PublishedBy string `gorm:"type:varchar(64);not null"`
	PublishedAt int64  `gorm:"autoCreateTime;index"`

	YankedAt     int64  `gorm:"not null;default:0;index"`
	YankedBy     string `gorm:"type:varchar(64);not null;default:''"`
	YankedReason string `gorm:"type:varchar(512);not null;default:''"`
}

func (pluginReleaseRow) TableName() string { return "plugin_release" }

func toPlugin(row *pluginRow) *model.Plugin {
	if row == nil {
		return nil
	}
	return &model.Plugin{
		Name:        row.Name,
		DisplayName: row.DisplayName,
		Description: row.Description,
		ArchivedAt:  row.ArchivedAt,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toPluginRelease(row *pluginReleaseRow) *model.PluginRelease {
	if row == nil {
		return nil
	}
	out := &model.PluginRelease{
		PluginName:         row.PluginName,
		Version:            row.Version,
		MinBuildmaxVersion: row.MinBuildmaxVersion,
		Digest:             row.Digest,
		ObjectKey:          row.ObjectKey,
		SizeBytes:          row.SizeBytes,
		PublishedBy:        row.PublishedBy,
		PublishedAt:        row.PublishedAt,
		YankedAt:           row.YankedAt,
		YankedBy:           row.YankedBy,
		YankedReason:       row.YankedReason,
	}
	// A document that will not decode costs the report, not the release: the
	// bytes and their digest are still exactly what was published.
	if row.Inspection != "" {
		_ = json.Unmarshal([]byte(row.Inspection), &out.Inspection)
	}
	if row.Source != "" {
		_ = json.Unmarshal([]byte(row.Source), &out.Source)
	}
	return out
}

// CreatePlugin adds a catalog entry.
func (s *Store) CreatePlugin(ctx context.Context, in model.CreatePluginInput) (*model.Plugin, error) {
	row := pluginRow{
		Name:        in.Name,
		DisplayName: in.DisplayName,
		Description: in.Description,
		CreatedBy:   in.CreatedBy,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isDuplicateKey(err) {
			return nil, model.ErrPluginNameTaken
		}
		return nil, err
	}
	return toPlugin(&row), nil
}

// GetPlugin returns one entry by name, or (nil, nil) when there is none.
func (s *Store) GetPlugin(ctx context.Context, name string) (*model.Plugin, error) {
	var row pluginRow
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toPlugin(&row), nil
}

// ListPlugins returns entries oldest first.
func (s *Store) ListPlugins(ctx context.Context, includeArchived bool) ([]model.Plugin, error) {
	q := s.db.WithContext(ctx).Order("created_at asc")
	if !includeArchived {
		q = q.Where("archived_at = 0")
	}
	var rows []pluginRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]model.Plugin, 0, len(rows))
	for i := range rows {
		out = append(out, *toPlugin(&rows[i]))
	}
	return out, nil
}

// UpdatePlugin changes display metadata.
func (s *Store) UpdatePlugin(ctx context.Context, name string, in model.UpdatePluginInput) (*model.Plugin, error) {
	res := s.db.WithContext(ctx).Model(&pluginRow{}).Where("name = ?", name).
		Updates(map[string]any{"display_name": in.DisplayName, "description": in.Description})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, model.ErrNotFound
	}
	return s.GetPlugin(ctx, name)
}

// SetPluginArchived retires or restores an entry.
func (s *Store) SetPluginArchived(ctx context.Context, name string, archived bool) error {
	var at int64
	if archived {
		at = time.Now().Unix()
	}
	res := s.db.WithContext(ctx).Model(&pluginRow{}).Where("name = ?", name).
		Update("archived_at", at)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return model.ErrNotFound
	}
	return nil
}

// CreatePluginRelease publishes one version.
//
// The unique index over (plugin_name, version) is the guard rather than a
// preceding read: two publishes racing would both pass a check and only one
// can pass the constraint.
func (s *Store) CreatePluginRelease(ctx context.Context, in model.CreatePluginReleaseInput) (*model.PluginRelease, error) {
	// The parent reference is read from the row: a catalog entry is addressed
	// by name everywhere above this package, so the model does not carry a
	// handle for it.
	var entry pluginRow
	err := s.db.WithContext(ctx).Where("name = ?", in.PluginName).Take(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if entry.ArchivedAt != 0 {
		return nil, model.ErrPluginArchived
	}

	inspection, err := json.Marshal(in.Inspection)
	if err != nil {
		return nil, fmt.Errorf("encode inspection: %w", err)
	}
	source, err := json.Marshal(in.Source)
	if err != nil {
		return nil, fmt.Errorf("encode source: %w", err)
	}

	row := pluginReleaseRow{
		PluginName:         entry.Name,
		Version:            in.Version,
		MinBuildmaxVersion: in.MinBuildmaxVersion,
		Digest:             in.Digest,
		ObjectKey:          in.ObjectKey,
		SizeBytes:          in.SizeBytes,
		Inspection:         string(inspection),
		Source:             string(source),
		PublishedBy:        in.PublishedBy,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isDuplicateKey(err) {
			return nil, model.ErrPluginVersionExists
		}
		return nil, err
	}
	return toPluginRelease(&row), nil
}

// GetPluginRelease returns one version, or (nil, nil) when there is none.
func (s *Store) GetPluginRelease(ctx context.Context, name, version string) (*model.PluginRelease, error) {
	var row pluginReleaseRow
	err := s.db.WithContext(ctx).Where("plugin_name = ? AND version = ?", name, version).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toPluginRelease(&row), nil
}

// ListPluginReleases returns every release of one plugin, oldest first.
//
// Yanked releases are included: which one to install is a decision that needs
// the version arithmetic, and a store that filtered here would hide the exact
// version a recovery asks for.
func (s *Store) ListPluginReleases(ctx context.Context, name string) ([]model.PluginRelease, error) {
	var rows []pluginReleaseRow
	err := s.db.WithContext(ctx).Where("plugin_name = ?", name).
		Order("published_at asc").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]model.PluginRelease, 0, len(rows))
	for i := range rows {
		out = append(out, *toPluginRelease(&rows[i]))
	}
	return out, nil
}

// YankPluginRelease withdraws a release from default selection.
//
// Yanking twice is not an error, but it does not rewrite who did it first: the
// record explains a past installation, so the first withdrawal is the fact.
func (s *Store) YankPluginRelease(ctx context.Context, name, version, actor, reason string) error {
	res := s.db.WithContext(ctx).Model(&pluginReleaseRow{}).
		Where("plugin_name = ? AND version = ? AND yanked_at = 0", name, version).
		Updates(map[string]any{
			"yanked_at":     time.Now().Unix(),
			"yanked_by":     actor,
			"yanked_reason": reason,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		existing, err := s.GetPluginRelease(ctx, name, version)
		if err != nil {
			return err
		}
		if existing == nil {
			return model.ErrNotFound
		}
	}
	return nil
}
