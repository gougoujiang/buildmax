package store

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestCreateUser(t *testing.T) {
	dsn := os.Getenv("BUILDMAX_TEST_DSN")
	if dsn == "" {
		t.Skip("BUILDMAX_TEST_DSN not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	email := "createuser-test@example.com"
	// Clean up if a previous run left the user.
	existing, _ := s.UserByEmail(ctx, email)
	if existing != nil {
		_ = s.db.WithContext(ctx).Delete(&User{}, "user_id = ?", existing.UserID)
	}

	u, err := s.CreateUser(ctx, email)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Email != email || u.UserID == "" {
		t.Errorf("CreateUser: got user %+v", u)
	}

	// UserByEmail should find the new user.
	found, err := s.UserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("UserByEmail: %v", err)
	}
	if found == nil || found.UserID != u.UserID {
		t.Errorf("UserByEmail: got %+v", found)
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	dsn := os.Getenv("BUILDMAX_TEST_DSN")
	if dsn == "" {
		t.Skip("BUILDMAX_TEST_DSN not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	email := "dup-test@example.com"
	u, err := s.CreateUser(ctx, email)
	if err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&User{}, "user_id = ?", u.UserID)
	}()

	_, err = s.CreateUser(ctx, email)
	if err == nil {
		t.Error("second CreateUser: expected error")
	}
	if !errors.Is(err, ErrEmailExists) {
		t.Errorf("second CreateUser: got %v, want ErrEmailExists", err)
	}
}
