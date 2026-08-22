package db

import (
	"context"
	"errors"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"gorm.io/gorm"
)

// artifactRow is the artifact table.
//
// The columns are deliberately bounded and queryable, with no free-form
// metadata JSON: durable metadata is exactly the place where prompts, file
// contents, and credentials leak in when a column will take anything. A new
// product behavior earns a new column.
type artifactRow struct {
	ID            uint   `gorm:"primaryKey;autoIncrement"`
	ArtifactID    string `gorm:"column:artifact_id;type:varchar(64);uniqueIndex;not null"`
	TeamID        string `gorm:"column:team_id;type:varchar(64);not null;index:idx_artifact_team_created,priority:1"`
	Filename      string `gorm:"type:varchar(512);not null"`
	MediaType     string `gorm:"column:media_type;type:varchar(255)"`
	SizeBytes     int64  `gorm:"column:size_bytes;not null"`
	SHA256        string `gorm:"column:sha256;type:varchar(64);not null"`
	StorageKey    string `gorm:"column:storage_key;type:varchar(1024);not null"`
	CreatedByType string `gorm:"column:created_by_type;type:varchar(32);not null"`
	CreatedByID   string `gorm:"column:created_by_id;type:varchar(64)"`
	SourceType    string `gorm:"column:source_type;type:varchar(32);not null"`
	SourceID      string `gorm:"column:source_id;type:varchar(64);index"`
	Title         string `gorm:"type:varchar(255)"`
	DeletedAt     *int64 `gorm:"column:deleted_at;index"`
	ExpiresAt     *int64 `gorm:"column:expires_at;index"`
	CreatedAt     int64  `gorm:"autoCreateTime;index:idx_artifact_team_created,priority:2"`
}

func (artifactRow) TableName() string { return "artifact" }

func toArtifact(row *artifactRow) *model.Artifact {
	if row == nil {
		return nil
	}
	return &model.Artifact{
		ID:            row.ID,
		ArtifactID:    row.ArtifactID,
		TeamID:        row.TeamID,
		Filename:      row.Filename,
		MediaType:     row.MediaType,
		SizeBytes:     row.SizeBytes,
		SHA256:        row.SHA256,
		StorageKey:    row.StorageKey,
		CreatedByType: row.CreatedByType,
		CreatedByID:   row.CreatedByID,
		SourceType:    row.SourceType,
		SourceID:      row.SourceID,
		Title:         row.Title,
		DeletedAt:     row.DeletedAt,
		ExpiresAt:     row.ExpiresAt,
		CreatedAt:     row.CreatedAt,
	}
}

func toArtifacts(rows []artifactRow) []model.Artifact {
	out := make([]model.Artifact, len(rows))
	for i := range rows {
		out[i] = *toArtifact(&rows[i])
	}
	return out
}

// CreateArtifact records one artifact. The ID is supplied by the caller, which
// reserved it before streaming so the storage key could be derived from it.
func (s *Store) CreateArtifact(ctx context.Context, in model.CreateArtifactInput) (*model.Artifact, error) {
	row := artifactRow{
		ArtifactID:    in.ArtifactID,
		TeamID:        in.TeamID,
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
	return toArtifact(&row), nil
}

// GetArtifact returns the artifact including a tombstoned one; the caller
// decides what a deleted artifact means for its route.
func (s *Store) GetArtifact(ctx context.Context, artifactID string) (*model.Artifact, error) {
	var row artifactRow
	err := s.db.WithContext(ctx).Where("artifact_id = ?", artifactID).First(&row).Error
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
	scope := func(q *gorm.DB) *gorm.DB {
		return q.Where("team_id = ? AND deleted_at IS NULL", teamID)
	}
	var total int64
	if err := scope(s.db.WithContext(ctx).Model(&artifactRow{})).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []artifactRow
	q := scope(s.db.WithContext(ctx)).Order("created_at DESC, id DESC")
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
	var list []artifactRow
	err := s.db.WithContext(ctx).
		Where("source_id IN ? AND deleted_at IS NULL", sourceIDs).
		Order("created_at DESC, id DESC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string][]model.Artifact, len(sourceIDs))
	for i := range list {
		out[list[i].SourceID] = append(out[list[i].SourceID], *toArtifact(&list[i]))
	}
	return out, nil
}

// SoftDeleteArtifact tombstones the artifact, which is what makes deletion take
// effect at the authorization boundary immediately. Removing the object itself
// happens later under retention policy.
//
// The update is conditional on the row still being live, so two concurrent
// deletes produce one audit-worthy change and one no-op rather than two.
func (s *Store) SoftDeleteArtifact(ctx context.Context, artifactID string, deletedAt int64) (bool, error) {
	res := s.db.WithContext(ctx).Model(&artifactRow{}).
		Where("artifact_id = ? AND deleted_at IS NULL", artifactID).
		Update("deleted_at", deletedAt)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
