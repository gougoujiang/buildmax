package db

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

// artifactShareRow is the artifact_share table: one revocable public link.
//
// The token itself is never stored — only its SHA-256, the same as login codes
// and webhook keys — so a leaked database row cannot be turned back into a
// working link. artifact_id and team_id are the internal keys; the public
// handles are joined on read, like the artifact table's own.
type artifactShareRow struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement"`
	PublicID        string     `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_artifact_share_public_id;not null"`
	ArtifactID      uint64     `gorm:"column:artifact_id;not null;index:idx_artifact_share_artifact"`
	TeamID          uint64     `gorm:"column:team_id;not null;index:idx_artifact_share_team_created,priority:1"`
	TokenSHA256     string     `gorm:"column:token_sha256;type:char(64) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_artifact_share_token;not null"`
	CreatedByType   string     `gorm:"column:created_by_type;type:varchar(32);not null"`
	CreatedByID     string     `gorm:"column:created_by_id;type:varchar(64)"`
	ExpiresAt       *time.Time `gorm:"column:expires_at;index"`
	RevokedAt       *time.Time `gorm:"column:revoked_at"`
	RetrievalCount  int64      `gorm:"column:retrieval_count;not null"`
	LastRetrievedAt *time.Time `gorm:"column:last_retrieved_at"`
	CreatedAt       time.Time  `gorm:"autoCreateTime;index:idx_artifact_share_team_created,priority:2"`
}

func (artifactShareRow) TableName() string { return "artifact_share" }

func toShare(row *artifactShareRow, artifactPublicID, teamPublicID string) coreartifact.ArtifactShare {
	return coreartifact.ArtifactShare{
		ShareID:         row.PublicID,
		ArtifactID:      artifactPublicID,
		TeamID:          teamPublicID,
		CreatedByType:   row.CreatedByType,
		CreatedByID:     row.CreatedByID,
		ExpiresAt:       row.ExpiresAt,
		RevokedAt:       row.RevokedAt,
		RetrievalCount:  row.RetrievalCount,
		LastRetrievedAt: row.LastRetrievedAt,
		CreatedAt:       row.CreatedAt,
	}
}

// CreateArtifactShare records one link for an existing artifact.
func (s *Store) CreateArtifactShare(ctx context.Context, in coreartifact.CreateShareInput) (*coreartifact.ArtifactShare, error) {
	artifactKey, err := lookupKey(ctx, s.db, "artifact", in.ArtifactID)
	if err != nil {
		return nil, err
	}
	teamKey, err := lookupKey(ctx, s.db, "team", in.TeamID)
	if err != nil {
		return nil, err
	}
	row := artifactShareRow{
		ArtifactID:    artifactKey,
		TeamID:        teamKey,
		TokenSHA256:   in.TokenSHA256,
		CreatedByType: in.CreatedByType,
		CreatedByID:   in.CreatedByID,
		ExpiresAt:     in.ExpiresAt,
	}
	if err := createWithPublicID(ctx, s.db, "uq_artifact_share_public_id",
		func(id string) { row.PublicID = id }, &row); err != nil {
		return nil, err
	}
	share := toShare(&row, canonicalPublicID(in.ArtifactID), canonicalPublicID(in.TeamID))
	return &share, nil
}

// GetArtifactShareByTokenHash returns the share and its artifact by the token's
// hash, applying no liveness or tombstone filter — the caller decides what a
// revoked, expired, or deleted target means, so a public route can answer every
// non-live case with one 404 while a management view still sees a revoked link.
func (s *Store) GetArtifactShareByTokenHash(ctx context.Context, tokenHash string) (*coreartifact.ResolvedShare, error) {
	var row artifactShareRow
	err := s.db.WithContext(ctx).Where("token_sha256 = ?", tokenHash).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	artifactID, err := publicIDForKey(ctx, s.db, "artifact", row.ArtifactID)
	if errors.Is(err, apierr.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	art, err := s.GetArtifact(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	if art == nil {
		return nil, nil
	}
	return &coreartifact.ResolvedShare{
		Share:    toShare(&row, artifactID, art.TeamID),
		Artifact: *art,
	}, nil
}

// ListArtifactShares returns an artifact's links, newest first, including
// revoked and expired ones so the management view can show what was.
func (s *Store) ListArtifactShares(ctx context.Context, artifactID string) ([]coreartifact.ArtifactShare, error) {
	art, err := s.GetArtifact(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	if art == nil {
		return nil, nil
	}
	artifactKey, err := lookupKey(ctx, s.db, "artifact", artifactID)
	if err != nil {
		return nil, err
	}
	var rows []artifactShareRow
	if err := s.db.WithContext(ctx).
		Where("artifact_id = ?", artifactKey).
		Order("created_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]coreartifact.ArtifactShare, len(rows))
	for i := range rows {
		out[i] = toShare(&rows[i], art.ID, art.TeamID)
	}
	return out, nil
}

// RevokeArtifactShare withdraws a link if it belongs to the artifact and is not
// already revoked, conditional on both so a repeat revoke is a no-op and a
// share_id from another artifact cannot be revoked through this one.
func (s *Store) RevokeArtifactShare(ctx context.Context, artifactID, shareID string, revokedAt time.Time) (bool, error) {
	artifactKey, err := lookupKey(ctx, s.db, "artifact", artifactID)
	if errors.Is(err, apierr.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	id, ok := util.CanonicalPublicID(shareID)
	if !ok {
		return false, nil
	}
	res := s.db.WithContext(ctx).Model(&artifactShareRow{}).
		Where("public_id = ? AND artifact_id = ? AND revoked_at IS NULL", id, artifactKey).
		Update("revoked_at", revokedAt)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// RecordArtifactShareRetrieval increments the count and stamps the time. It is
// best-effort: a caller ignores its error rather than failing a download.
func (s *Store) RecordArtifactShareRetrieval(ctx context.Context, shareID string, at time.Time) error {
	id, ok := util.CanonicalPublicID(shareID)
	if !ok {
		return nil
	}
	return s.db.WithContext(ctx).Model(&artifactShareRow{}).
		Where("public_id = ?", id).
		Updates(map[string]any{
			"retrieval_count":   gorm.Expr("retrieval_count + 1"),
			"last_retrieved_at": at,
		}).Error
}
