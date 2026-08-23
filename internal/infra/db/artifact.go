package db

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

// artifactRow is the artifact table.
//
// The columns are deliberately bounded and queryable, with no free-form
// metadata JSON: durable metadata is exactly the place where prompts, file
// contents, and credentials leak in when a column will take anything. A new
// product behavior earns a new column.
type artifactRow struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID   string `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_artifact_public_id;not null"`
	TeamID     uint64 `gorm:"column:team_id;not null;index:idx_artifact_team_created,priority:1"`
	Filename   string `gorm:"type:varchar(512);not null"`
	MediaType  string `gorm:"column:media_type;type:varchar(255)"`
	SizeBytes  int64  `gorm:"column:size_bytes;not null"`
	SHA256     string `gorm:"column:sha256;type:varchar(64);not null"`
	StorageKey string `gorm:"column:storage_key;type:varchar(1024);not null"`
	// The creator and the source stay opaque handles under their type columns:
	// an artifact can come from a run, a conversation, or an upload, and the
	// producer can be a worker rather than a user.
	CreatedByType string     `gorm:"column:created_by_type;type:varchar(32);not null"`
	CreatedByID   string     `gorm:"column:created_by_id;type:varchar(64)"`
	SourceType    string     `gorm:"column:source_type;type:varchar(32);not null"`
	SourceID      string     `gorm:"column:source_id;type:varchar(64);index"`
	Title         string     `gorm:"type:varchar(255)"`
	DeletedAt     *time.Time `gorm:"column:deleted_at;index"`
	ExpiresAt     *time.Time `gorm:"column:expires_at;index"`
	CreatedAt     time.Time  `gorm:"autoCreateTime;index:idx_artifact_team_created,priority:2"`
}

func (artifactRow) TableName() string { return "artifact" }

// artifactReadRow is the row plus its team's handle.
type artifactReadRow struct {
	Row          artifactRow `gorm:"embedded"`
	TeamPublicID string      `gorm:"column:team_public_id"`
}

func (s *Store) artifactSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&artifactRow{}).
		Select("artifact.*, t.public_id AS team_public_id").
		Joins("INNER JOIN team t ON t.id = artifact.team_id")
}

func toArtifact(row *artifactReadRow) *model.Artifact {
	if row == nil {
		return nil
	}
	return &model.Artifact{
		ID:            row.Row.PublicID,
		TeamID:        row.TeamPublicID,
		Filename:      row.Row.Filename,
		MediaType:     row.Row.MediaType,
		SizeBytes:     row.Row.SizeBytes,
		SHA256:        row.Row.SHA256,
		StorageKey:    row.Row.StorageKey,
		CreatedByType: row.Row.CreatedByType,
		CreatedByID:   row.Row.CreatedByID,
		SourceType:    row.Row.SourceType,
		SourceID:      row.Row.SourceID,
		Title:         row.Row.Title,
		DeletedAt:     row.Row.DeletedAt,
		ExpiresAt:     row.Row.ExpiresAt,
		CreatedAt:     row.Row.CreatedAt,
	}
}

func toArtifacts(rows []artifactReadRow) []model.Artifact {
	out := make([]model.Artifact, len(rows))
	for i := range rows {
		out[i] = *toArtifact(&rows[i])
	}
	return out
}

// CreateArtifact records one artifact. The ID is supplied by the caller, which
// reserved it before streaming so the storage key could be derived from it.
func (s *Store) CreateArtifact(ctx context.Context, in model.CreateArtifactInput) (*model.Artifact, error) {
	teamKey, err := lookupKey(ctx, s.db, "team", in.TeamID)
	if err != nil {
		return nil, err
	}
	id, ok := util.CanonicalPublicID(in.ArtifactID)
	if !ok {
		return nil, model.ErrNotFound
	}
	row := artifactRow{
		PublicID:      id,
		TeamID:        teamKey,
		Filename:      in.Filename,
		MediaType:     in.MediaType,
		SizeBytes:     in.SizeBytes,
		SHA256:        in.SHA256,
		StorageKey:    in.StorageKey,
		CreatedByType: in.CreatedByType,
		CreatedByID:   in.CreatedByID,
		SourceType:    in.SourceType,
		SourceID:      in.SourceID,
		Title:         in.Title,
		ExpiresAt:     in.ExpiresAt,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return toArtifact(&artifactReadRow{Row: row, TeamPublicID: canonicalPublicID(in.TeamID)}), nil
}

// GetArtifact returns the artifact including a tombstoned one; the caller
// decides what a deleted artifact means for its route.
func (s *Store) GetArtifact(ctx context.Context, artifactID string) (*model.Artifact, error) {
	id, ok := util.CanonicalPublicID(artifactID)
	if !ok {
		return nil, nil
	}
	var row artifactReadRow
	err := s.artifactSelect(ctx).Where("artifact.public_id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toArtifact(&row), nil
}

// ListArtifactsByTeam returns the team's live artifacts, newest first.
func (s *Store) ListArtifactsByTeam(ctx context.Context, teamID string, limit, offset int) ([]model.Artifact, int, error) {
	limit, offset = capPage(limit, offset)
	teamKey, err := lookupKey(ctx, s.db, "team", teamID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&artifactRow{}).
		Where("team_id = ? AND deleted_at IS NULL", teamKey).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []artifactReadRow
	q := s.artifactSelect(ctx).Where("artifact.team_id = ? AND artifact.deleted_at IS NULL", teamKey).
		Order("artifact.created_at DESC, artifact.id DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return toArtifacts(list), int(total), nil
}

// ListArtifactsBySource groups a set of producing operations' artifacts.
//
// Sources with nothing published are absent from the map rather than present
// with an empty slice, so a caller has to treat a miss as "produced none".
func (s *Store) ListArtifactsBySource(ctx context.Context, sourceIDs []string) (map[string][]model.Artifact, error) {
	if len(sourceIDs) == 0 {
		return map[string][]model.Artifact{}, nil
	}
	var list []artifactReadRow
	err := s.artifactSelect(ctx).
		Where("artifact.source_id IN ? AND artifact.deleted_at IS NULL", sourceIDs).
		Order("artifact.created_at DESC, artifact.id DESC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string][]model.Artifact, len(sourceIDs))
	for i := range list {
		out[list[i].Row.SourceID] = append(out[list[i].Row.SourceID], *toArtifact(&list[i]))
	}
	return out, nil
}

// SoftDeleteArtifact tombstones the artifact, which is what makes deletion take
// effect at the authorization boundary immediately. Removing the object itself
// happens later under retention policy.
//
// The update is conditional on the row still being live, so two concurrent
// deletes produce one audit-worthy change and one no-op rather than two.
func (s *Store) SoftDeleteArtifact(ctx context.Context, artifactID string, deletedAt time.Time) (bool, error) {
	id, ok := util.CanonicalPublicID(artifactID)
	if !ok {
		return false, nil
	}
	res := s.db.WithContext(ctx).Model(&artifactRow{}).
		Where("public_id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", deletedAt)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
