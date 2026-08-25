package db

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"

	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

type conversationRow struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID  string `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_conversation_public_id;not null"`
	UserID    uint64 `gorm:"column:user_id;not null;index:idx_conversation_user_created,priority:1"`
	TeamID    uint64 `gorm:"column:team_id;index:idx_conversation_team_created,priority:1"`
	Channel   string `gorm:"type:varchar(32);not null"`
	Title     string `gorm:"type:varchar(256)"`
	CreatedBy uint64 `gorm:"column:created_by;not null"`
	// The two composite indexes carry created_at because every listing of a
	// conversation is ordered by it. The single-column indexes the string model
	// left behind could not serve the sort.
	CreatedAt time.Time `gorm:"autoCreateTime;index:idx_conversation_user_created,priority:2;index:idx_conversation_team_created,priority:2"`
}

func (conversationRow) TableName() string { return "conversation" }

// conversationReadRow is the row plus the handles its references resolve to.
// The row is a named field: see teamReadRow for why anonymous embedding does
// not work here. A pointer field is one a LEFT JOIN may leave NULL.
type conversationReadRow struct {
	Row               conversationRow `gorm:"embedded"`
	UserPublicID      string          `gorm:"column:user_public_id"`
	TeamPublicID      *string         `gorm:"column:team_public_id"`
	CreatedByPublicID string          `gorm:"column:created_by_public_id"`
}

func (s *Store) conversationSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&conversationRow{}).
		Select("conversation.*, u.public_id AS user_public_id, t.public_id AS team_public_id, cb.public_id AS created_by_public_id").
		Joins("INNER JOIN `user` u ON u.id = conversation.user_id").
		Joins("LEFT JOIN team t ON t.id = conversation.team_id").
		Joins("INNER JOIN `user` cb ON cb.id = conversation.created_by")
}

func toConversation(row *conversationReadRow) *model.Conversation {
	if row == nil {
		return nil
	}
	return &model.Conversation{
		ID:        row.Row.PublicID,
		UserID:    row.UserPublicID,
		TeamID:    derefPublicID(row.TeamPublicID),
		Channel:   row.Row.Channel,
		Title:     row.Row.Title,
		CreatedBy: row.CreatedByPublicID,
		CreatedAt: row.Row.CreatedAt,
	}
}

func toConversations(rows []conversationReadRow) []model.Conversation {
	out := make([]model.Conversation, len(rows))
	for i := range rows {
		out[i] = *toConversation(&rows[i])
	}
	return out
}

// CreateConversation creates a new Tier 1 conversation. Returns the conversation with conversation_id set.
func (s *Store) CreateConversation(ctx context.Context, userID, channel, createdBy string) (*model.Conversation, error) {
	teamID, err := s.personalTeamIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.CreateConversationInTeam(ctx, teamID, userID, channel, createdBy)
}

// CreateConversationInTeam creates a new team-scoped Tier 1 conversation.
func (s *Store) CreateConversationInTeam(ctx context.Context, teamID, userID, channel, createdBy string) (*model.Conversation, error) {
	now := time.Now().UTC()
	row := &conversationRow{Channel: channel, CreatedAt: now}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		userKey, err := lookupKey(ctx, tx, "user", userID)
		if err != nil {
			return err
		}
		row.UserID = userKey
		if teamID != "" {
			teamKey, err := lookupKey(ctx, tx, "team", teamID)
			if err != nil {
				return err
			}
			row.TeamID = teamKey
		}
		creator, err := lookupKey(ctx, tx, "user", createdBy)
		if err != nil {
			return err
		}
		row.CreatedBy = creator
		return createWithPublicID(ctx, tx, "uq_conversation_public_id",
			func(id string) { row.PublicID = id }, row)
	}); err != nil {
		return nil, err
	}
	return &model.Conversation{
		ID:        row.PublicID,
		UserID:    userID,
		TeamID:    teamID,
		Channel:   channel,
		CreatedBy: createdBy,
		CreatedAt: now,
	}, nil
}

// GetConversation returns the conversation by conversation_id, or (nil, nil) if not found.
func (s *Store) GetConversation(ctx context.Context, conversationID string) (*model.Conversation, error) {
	id, ok := util.CanonicalPublicID(conversationID)
	if !ok {
		return nil, nil
	}
	var c conversationReadRow
	err := s.conversationSelect(ctx).Where("conversation.public_id = ?", id).Take(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toConversation(&c), nil
}

// ListConversationsByUser returns conversations for the user ordered by created_at DESC.
// total is the total count of matching conversations (ignoring limit/offset).
func (s *Store) ListConversationsByUser(ctx context.Context, userID string, limit, offset int) ([]model.Conversation, int, error) {
	limit, offset = capPage(limit, offset)
	userKey, err := lookupKey(ctx, s.db, "user", userID)
	if errors.Is(err, apierr.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&conversationRow{}).Where("user_id = ?", userKey).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []conversationReadRow
	err = s.conversationSelect(ctx).Where("conversation.user_id = ?", userKey).
		Order("conversation.created_at DESC").
		Limit(limit).Offset(offset).
		Find(&list).Error
	return toConversations(list), int(total), err
}

// ListConversationsByTeam returns the team's conversations, newest first.
//
// Synthetic conversations are excluded. A workflow step and an issue agent run
// each create one because Task requires a conversation, not because anyone is
// talking through it, and a team that runs either would otherwise find its own
// conversations pushed off the first page by machinery. They are still there
// and a link straight to one still opens it — see model.SyntheticChannels.
//
// The filter is here rather than in the caller because the page is cut here:
// dropping the rows after the limit would leave a short page with a total that
// disagrees with it.
func (s *Store) ListConversationsByTeam(ctx context.Context, teamID string, limit, offset int) ([]model.Conversation, int, error) {
	limit, offset = capPage(limit, offset)
	teamKey, err := lookupKey(ctx, s.db, "team", teamID)
	if errors.Is(err, apierr.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&conversationRow{}).
		Where("team_id = ? AND channel NOT IN ?", teamKey, model.SyntheticChannels()).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []conversationReadRow
	err = s.conversationSelect(ctx).
		Where("conversation.team_id = ? AND conversation.channel NOT IN ?", teamKey, model.SyntheticChannels()).
		Order("conversation.created_at DESC").
		Limit(limit).Offset(offset).
		Find(&list).Error
	return toConversations(list), int(total), err
}

// UpdateConversationTitle sets the title for the conversation (e.g. after first-round LLM generation).
func (s *Store) UpdateConversationTitle(ctx context.Context, conversationID, title string) error {
	id, ok := util.CanonicalPublicID(conversationID)
	if !ok {
		return apierr.ErrNotFound
	}
	return s.db.WithContext(ctx).Model(&conversationRow{}).
		Where("public_id = ?", id).
		Update("title", title).Error
}
