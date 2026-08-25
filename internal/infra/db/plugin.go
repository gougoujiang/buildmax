package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"
)

// pluginRow is one catalog entry. It has no team column: the catalog belongs to
// the deployment, which is what lets a System Administrator manage company
// capabilities without reaching into any team's content.
type pluginRow struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	Name        string     `gorm:"type:varchar(128);uniqueIndex;not null"`
	DisplayName string     `gorm:"type:varchar(255);not null;default:''"`
	Description string     `gorm:"type:varchar(1024);not null;default:''"`
	ArchivedAt  *time.Time `gorm:"index"`
	CreatedBy   uint64     `gorm:"column:created_by;not null"`
	CreatedAt   time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime"`
}

func (pluginRow) TableName() string { return "plugin" }

// pluginReleaseRow is one published version.
//
// Inspection and Source are JSON documents rather than columns because nothing
// queries inside them: they are written once and read whole, and giving each
// field a column would freeze the report's shape into the schema.
type pluginReleaseRow struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"`
	// PluginID is the parent reference. PluginName is denormalised beside it so
	// a release reads without a join, and the unique index over
	// (plugin_name, version) is what makes a version immutable.
	PluginID           uint64 `gorm:"column:plugin_id;not null;index"`
	PluginName         string `gorm:"type:varchar(128);not null;uniqueIndex:ux_plugin_release_version,priority:1"`
	Version            string `gorm:"type:varchar(64);not null;uniqueIndex:ux_plugin_release_version,priority:2"`
	MinBuildmaxVersion string `gorm:"type:varchar(64);not null;default:''"`

	Digest    string `gorm:"type:varchar(128);not null;index"`
	ObjectKey string `gorm:"type:varchar(512);not null"`
	SizeBytes int64  `gorm:"not null;default:0"`

	Inspection string `gorm:"type:text"`
	Source     string `gorm:"type:text"`

	PublishedBy uint64    `gorm:"column:published_by;not null"`
	PublishedAt time.Time `gorm:"autoCreateTime;index"`

	YankedAt     *time.Time `gorm:"index"`
	YankedBy     *uint64    `gorm:"column:yanked_by"`
	YankedReason string     `gorm:"type:varchar(512);not null;default:''"`
}

func (pluginReleaseRow) TableName() string { return "plugin_release" }

// pluginReadRow and pluginReleaseReadRow carry the handles of the users a
// catalog record names. Neither record has a handle of its own: an entry is
// addressed by name and a release by name plus version. A pointer field is one
// a LEFT JOIN may leave NULL.
type pluginReadRow struct {
	Row               pluginRow `gorm:"embedded"`
	CreatedByPublicID string    `gorm:"column:created_by_public_id"`
}

type pluginReleaseReadRow struct {
	Row                 pluginReleaseRow `gorm:"embedded"`
	PublishedByPublicID string           `gorm:"column:published_by_public_id"`
	YankedByPublicID    *string          `gorm:"column:yanked_by_public_id"`
}

func (s *Store) pluginSelect(ctx context.Context) *gorm.DB {
	return pluginSelectTx(s.db.WithContext(ctx))
}

func pluginSelectTx(tx *gorm.DB) *gorm.DB {
	return tx.Model(&pluginRow{}).
		Select("plugin.*, cb.public_id AS created_by_public_id").
		Joins("INNER JOIN `user` cb ON cb.id = plugin.created_by")
}

func (s *Store) pluginReleaseSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&pluginReleaseRow{}).
		Select("plugin_release.*, pb.public_id AS published_by_public_id, yb.public_id AS yanked_by_public_id").
		Joins("INNER JOIN `user` pb ON pb.id = plugin_release.published_by").
		Joins("LEFT JOIN `user` yb ON yb.id = plugin_release.yanked_by")
}

func toPlugin(row *pluginReadRow) *model.Plugin {
	if row == nil {
		return nil
	}
	return &model.Plugin{
		Name:        row.Row.Name,
		DisplayName: row.Row.DisplayName,
		Description: row.Row.Description,
		ArchivedAt:  row.Row.ArchivedAt,
		CreatedBy:   row.CreatedByPublicID,
		CreatedAt:   row.Row.CreatedAt,
		UpdatedAt:   row.Row.UpdatedAt,
	}
}

func toPluginRelease(row *pluginReleaseReadRow) *model.PluginRelease {
	if row == nil {
		return nil
	}
	out := &model.PluginRelease{
		PluginName:         row.Row.PluginName,
		Version:            row.Row.Version,
		MinBuildmaxVersion: row.Row.MinBuildmaxVersion,
		Digest:             row.Row.Digest,
		ObjectKey:          row.Row.ObjectKey,
		SizeBytes:          row.Row.SizeBytes,
		PublishedBy:        row.PublishedByPublicID,
		PublishedAt:        row.Row.PublishedAt,
		YankedAt:           row.Row.YankedAt,
		YankedBy:           derefPublicID(row.YankedByPublicID),
		YankedReason:       row.Row.YankedReason,
	}
	// A document that will not decode costs the report, not the release: the
	// bytes and their digest are still exactly what was published.
	if row.Row.Inspection != "" {
		_ = json.Unmarshal([]byte(row.Row.Inspection), &out.Inspection)
	}
	if row.Row.Source != "" {
		_ = json.Unmarshal([]byte(row.Row.Source), &out.Source)
	}
	return out
}

// CreatePlugin adds a catalog entry.
func (s *Store) CreatePlugin(ctx context.Context, in model.CreatePluginInput) (*model.Plugin, error) {
	creator, err := lookupKey(ctx, s.db, "user", in.CreatedBy)
	if err != nil {
		return nil, err
	}
	row := pluginRow{
		CreatedBy:   creator,
		Name:        in.Name,
		DisplayName: in.DisplayName,
		Description: in.Description,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isDuplicateKey(err) {
			return nil, model.ErrPluginNameTaken
		}
		return nil, err
	}
	return toPlugin(&pluginReadRow{Row: row, CreatedByPublicID: canonicalPublicID(in.CreatedBy)}), nil
}

// GetPlugin returns one entry by name, or (nil, nil) when there is none.
func (s *Store) GetPlugin(ctx context.Context, name string) (*model.Plugin, error) {
	var row pluginReadRow
	err := s.pluginSelect(ctx).Where("plugin.name = ?", name).Take(&row).Error
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
	q := s.pluginSelect(ctx).Order("plugin.created_at asc")
	if !includeArchived {
		q = q.Where("plugin.archived_at IS NULL")
	}
	var rows []pluginReadRow
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
		return nil, apierr.ErrNotFound
	}
	return s.GetPlugin(ctx, name)
}

// SetPluginArchived retires or restores an entry.
func (s *Store) SetPluginArchived(ctx context.Context, name string, archived bool) error {
	var at *time.Time
	if archived {
		now := time.Now().UTC()
		at = &now
	}
	res := s.db.WithContext(ctx).Model(&pluginRow{}).Where("name = ?", name).
		Update("archived_at", at)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apierr.ErrNotFound
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
		return nil, apierr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if entry.ArchivedAt != nil {
		return nil, model.ErrPluginArchived
	}

	publisher, err := lookupKey(ctx, s.db, "user", in.PublishedBy)
	if err != nil {
		return nil, err
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
		PublishedBy:        publisher,
		PluginID:           entry.ID,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isDuplicateKey(err) {
			return nil, model.ErrPluginVersionExists
		}
		return nil, err
	}
	return toPluginRelease(&pluginReleaseReadRow{
		Row: row, PublishedByPublicID: canonicalPublicID(in.PublishedBy),
	}), nil
}

// GetPluginRelease returns one version, or (nil, nil) when there is none.
func (s *Store) GetPluginRelease(ctx context.Context, name, version string) (*model.PluginRelease, error) {
	var row pluginReleaseReadRow
	err := s.pluginReleaseSelect(ctx).
		Where("plugin_release.plugin_name = ? AND plugin_release.version = ?", name, version).Take(&row).Error
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
	var rows []pluginReleaseReadRow
	err := s.pluginReleaseSelect(ctx).Where("plugin_release.plugin_name = ?", name).
		Order("plugin_release.published_at asc").Find(&rows).Error
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
	yankedBy, err := lookupKey(ctx, s.db, "user", actor)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).Model(&pluginReleaseRow{}).
		Where("plugin_name = ? AND version = ? AND yanked_at IS NULL", name, version).
		Updates(map[string]any{
			"yanked_at":     time.Now().UTC(),
			"yanked_by":     yankedBy,
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
			return apierr.ErrNotFound
		}
	}
	return nil
}
