// Package store provides user persistence (MySQL via GORM).
package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// User is the user model. JSON uses snake_case per project convention.
type User struct {
	ID        string `gorm:"type:varchar(36);primaryKey" json:"id"`
	Email     string `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	Name      string `gorm:"type:varchar(255)" json:"name"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM.
func (User) TableName() string {
	return "users"
}

// UserStore looks up users by email.
type UserStore interface {
	UserByEmail(ctx context.Context, email string) (*User, error)
}

// Store implements UserStore with a MySQL backend.
type Store struct {
	db *gorm.DB
}

// New opens a MySQL connection with the given DSN and runs AutoMigrate for User.
// The context can be used for connection timeout; the returned Store holds the DB for the process lifetime.
func New(ctx context.Context, dsn string) (*Store, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.WithContext(ctx).AutoMigrate(&User{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// UserByEmail returns the user with the given email, or (nil, nil) when not found.
func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// Close closes the underlying DB connection. Optional for server lifecycle.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
