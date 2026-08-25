package db

import (
	"context"
	"errors"
	"maps"
	"time"

	"gorm.io/gorm"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"
)

// pluginActivationRow is one team's pinned use of one catalog plugin.
//
// The plugin is referenced by name, as a release is: a catalog entry is
// addressed by name everywhere above this package. The unique index over
// (team_id, plugin_name) is what makes an activation one row per pair rather
// than a history — suspension is a flag on that row, so the pin survives it.
type pluginActivationRow struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID string `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_plugin_activation_public_id;not null"`

	TeamID     uint64 `gorm:"column:team_id;not null;uniqueIndex:ux_plugin_activation_team_plugin,priority:1"`
	PluginName string `gorm:"type:varchar(128);not null;uniqueIndex:ux_plugin_activation_team_plugin,priority:2"`

	Version string `gorm:"type:varchar(64);not null"`
	Digest  string `gorm:"type:varchar(128);not null"`
	Enabled bool   `gorm:"not null;default:true"`
	Origin  string `gorm:"type:varchar(16);not null;default:'curated'"`

	ActivatedBy uint64    `gorm:"column:activated_by;not null"`
	ActivatedAt time.Time `gorm:"autoCreateTime;index"`
	UpdatedBy   uint64    `gorm:"column:updated_by;not null"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

func (pluginActivationRow) TableName() string { return "plugin_activation" }

// pluginActivationReadRow is the row plus the handles its references resolve
// to. A pointer field is one a LEFT JOIN may leave NULL.
type pluginActivationReadRow struct {
	Row                 pluginActivationRow `gorm:"embedded"`
	TeamPublicID        string              `gorm:"column:team_public_id"`
	ActivatedByPublicID string              `gorm:"column:activated_by_public_id"`
	UpdatedByPublicID   *string             `gorm:"column:updated_by_public_id"`
}

func (s *Store) pluginActivationSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&pluginActivationRow{}).
		Select("plugin_activation.*, t.public_id AS team_public_id, ab.public_id AS activated_by_public_id, ub.public_id AS updated_by_public_id").
		Joins("INNER JOIN team t ON t.id = plugin_activation.team_id").
		Joins("INNER JOIN `user` ab ON ab.id = plugin_activation.activated_by").
		Joins("LEFT JOIN `user` ub ON ub.id = plugin_activation.updated_by")
}

func toPluginActivation(row *pluginActivationReadRow) *model.PluginActivation {
	if row == nil {
		return nil
	}
	return &model.PluginActivation{
		ID:          row.Row.PublicID,
		TeamID:      row.TeamPublicID,
		PluginName:  row.Row.PluginName,
		Version:     row.Row.Version,
		Digest:      row.Row.Digest,
		Enabled:     row.Row.Enabled,
		Origin:      model.PluginActivationOrigin(row.Row.Origin),
		ActivatedBy: row.ActivatedByPublicID,
		ActivatedAt: row.Row.ActivatedAt,
		UpdatedBy:   derefPublicID(row.UpdatedByPublicID),
		UpdatedAt:   row.Row.UpdatedAt,
	}
}

// ActivatePlugin records a new activation.
//
// The unique index is the guard rather than a preceding read: two admins
// activating the same plugin would both pass a check and only one can pass the
// constraint.
func (s *Store) ActivatePlugin(ctx context.Context, in model.ActivatePluginInput) (*model.PluginActivation, error) {
	team, err := lookupKey(ctx, s.db, "team", in.TeamID)
	if err != nil {
		return nil, err
	}
	actor, err := lookupKey(ctx, s.db, "user", in.ActorID)
	if err != nil {
		return nil, err
	}
	row := pluginActivationRow{
		TeamID:      team,
		PluginName:  in.PluginName,
		Version:     in.Version,
		Digest:      in.Digest,
		Enabled:     true,
		Origin:      string(in.Origin),
		ActivatedBy: actor,
		UpdatedBy:   actor,
	}
	err = createWithPublicID(ctx, s.db, "uq_plugin_activation_public_id",
		func(id string) { row.PublicID = id }, &row)
	if err != nil {
		if isDuplicateKey(err) {
			return nil, model.ErrPluginAlreadyActivated
		}
		return nil, err
	}
	return s.GetPluginActivation(ctx, in.TeamID, in.PluginName)
}

// GetPluginActivation returns one team's activation of one plugin.
func (s *Store) GetPluginActivation(ctx context.Context, teamID, pluginName string) (*model.PluginActivation, error) {
	team, err := lookupKey(ctx, s.db, "team", teamID)
	if err != nil {
		return nil, err
	}
	var row pluginActivationReadRow
	err = s.pluginActivationSelect(ctx).
		Where("plugin_activation.team_id = ? AND plugin_activation.plugin_name = ?", team, pluginName).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toPluginActivation(&row), nil
}

// ListPluginActivations returns a team's activations, oldest first.
func (s *Store) ListPluginActivations(ctx context.Context, teamID string) ([]model.PluginActivation, error) {
	team, err := lookupKey(ctx, s.db, "team", teamID)
	if err != nil {
		return nil, err
	}
	var rows []pluginActivationReadRow
	err = s.pluginActivationSelect(ctx).
		Where("plugin_activation.team_id = ?", team).
		Order("plugin_activation.activated_at asc").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]model.PluginActivation, 0, len(rows))
	for i := range rows {
		out = append(out, *toPluginActivation(&rows[i]))
	}
	return out, nil
}

// MovePluginActivationPin repoints an activation at another release.
func (s *Store) MovePluginActivationPin(ctx context.Context, in model.MovePluginActivationPinInput) (*model.PluginActivation, error) {
	return s.updatePluginActivation(ctx, in.TeamID, in.PluginName, in.ActorID, map[string]any{
		"version": in.Version,
		"digest":  in.Digest,
	})
}

// SetPluginActivationEnabled suspends or resumes an activation.
func (s *Store) SetPluginActivationEnabled(ctx context.Context, teamID, pluginName string, enabled bool, actorID string) (*model.PluginActivation, error) {
	return s.updatePluginActivation(ctx, teamID, pluginName, actorID, map[string]any{
		"enabled": enabled,
	})
}

// updatePluginActivation applies fields to one activation and stamps who did
// it. Every write here records an actor: an activation whose last change has no
// name answers half the question it exists to answer.
func (s *Store) updatePluginActivation(ctx context.Context, teamID, pluginName, actorID string, fields map[string]any) (*model.PluginActivation, error) {
	team, err := lookupKey(ctx, s.db, "team", teamID)
	if err != nil {
		return nil, err
	}
	actor, err := lookupKey(ctx, s.db, "user", actorID)
	if err != nil {
		return nil, err
	}
	updates := make(map[string]any, len(fields)+1)
	maps.Copy(updates, fields)
	updates["updated_by"] = actor

	res := s.db.WithContext(ctx).Model(&pluginActivationRow{}).
		Where("team_id = ? AND plugin_name = ?", team, pluginName).
		Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// Zero rows is either no such activation or an update that changed
		// nothing. Reading tells the two apart, and re-suspending an already
		// suspended activation must not read as not found.
		existing, getErr := s.GetPluginActivation(ctx, teamID, pluginName)
		if getErr != nil {
			return nil, getErr
		}
		if existing == nil {
			return nil, apierr.ErrNotFound
		}
		return existing, nil
	}
	return s.GetPluginActivation(ctx, teamID, pluginName)
}
