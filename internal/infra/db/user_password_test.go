package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

func TestPasswordLifecycle(t *testing.T) {
	s, ctx := newTestStore(t)
	email := "passwordtest@example.com"
	t.Cleanup(func() {
		_ = s.db.WithContext(context.Background()).Delete(&userRow{}, "email = ?", email)
	})

	user, err := s.CreateUser(ctx, email, "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_ = s.db.WithContext(bg).Delete(&teamMemberRow{}, "user_id = ?", user.ID)
		_ = s.db.WithContext(bg).Delete(&teamRow{}, "personal_for_user_id = ?", user.ID)
	})

	// A new account has no password, which is what sends its owner to a login
	// code rather than to a sign-in form they cannot satisfy.
	hash, err := s.PasswordHash(ctx, user.ID)
	if err != nil {
		t.Fatalf("PasswordHash: %v", err)
	}
	if hash != "" {
		t.Errorf("a new account already has a password hash: %q", hash)
	}
	if user.HasPassword {
		t.Error("HasPassword is true for an account that has never set one")
	}

	encoded, err := model.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := s.SetPassword(ctx, user.ID, encoded, time.Now().Unix()); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	stored, err := s.PasswordHash(ctx, user.ID)
	if err != nil {
		t.Fatalf("PasswordHash: %v", err)
	}
	if !model.VerifyPassword(stored, "correct horse battery staple") {
		t.Error("the stored hash does not verify the password it was made from")
	}

	// HasPassword follows, and the hash itself does not ride along on the user.
	reloaded, err := s.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !reloaded.HasPassword {
		t.Error("HasPassword is false after setting one")
	}
}

// The column is varchar(255); an argon2id hash must fit with room to spare, or
// MySQL would truncate it and every login would fail after a silent corruption.
func TestPasswordHashFitsItsColumn(t *testing.T) {
	s, ctx := newTestStore(t)
	email := "passwordwidth@example.com"
	t.Cleanup(func() {
		_ = s.db.WithContext(context.Background()).Delete(&userRow{}, "email = ?", email)
	})
	user, err := s.CreateUser(ctx, email, "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_ = s.db.WithContext(bg).Delete(&teamMemberRow{}, "user_id = ?", user.ID)
		_ = s.db.WithContext(bg).Delete(&teamRow{}, "personal_for_user_id = ?", user.ID)
	})

	const password = "a password at the long end of what anyone would actually type"
	encoded, err := model.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := s.SetPassword(ctx, user.ID, encoded, time.Now().Unix()); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	stored, err := s.PasswordHash(ctx, user.ID)
	if err != nil {
		t.Fatalf("PasswordHash: %v", err)
	}
	if stored != encoded {
		t.Fatalf("the stored hash differs from what was written (%d vs %d bytes)", len(stored), len(encoded))
	}
	if !model.VerifyPassword(stored, password) {
		t.Error("a round-tripped hash does not verify")
	}
}

func TestSetPasswordRejectsAnUnknownAccount(t *testing.T) {
	s, ctx := newTestStore(t)
	encoded, err := model.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := s.SetPassword(ctx, "u_doesnotexist", encoded, time.Now().Unix()); !errors.Is(err, model.ErrUserNotFound) {
		t.Errorf("SetPassword = %v, want ErrUserNotFound", err)
	}
}

// An unknown account and one with no password are the same answer, so a caller
// cannot use this to ask which addresses are registered.
func TestPasswordHashOfAnUnknownAccountIsEmptyNotAnError(t *testing.T) {
	s, ctx := newTestStore(t)
	hash, err := s.PasswordHash(ctx, "u_doesnotexist")
	if err != nil {
		t.Fatalf("PasswordHash: %v", err)
	}
	if hash != "" {
		t.Errorf("hash = %q, want empty", hash)
	}
}
