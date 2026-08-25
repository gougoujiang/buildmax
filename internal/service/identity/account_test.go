package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/identity"
)

// TestSetPasswordChecksTheRulesBeforeTheCurrentPassword pins an order that is
// about what a person is told, not about correctness.
//
// Somebody typing a new password that could never be accepted should hear which
// rule they missed. Checking the current one first tells them instead that
// their current password is wrong, which sends them to reset a password that
// was never the problem.
func TestSetPasswordChecksTheRulesBeforeTheCurrentPassword(t *testing.T) {
	hash, err := coreidentity.HashPassword(goodPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	svc := &identity.Service{
		Users:     &mock.MockUserStore{ByID: map[string]*coreidentity.User{"u_1": {ID: "u_1"}}},
		Passwords: &mock.MockPasswordStore{Hashes: map[string]string{"u_1": hash}},
	}
	// Both wrong: the new password is too short and the current one is not the
	// stored one.
	err = svc.SetPassword(context.Background(), identity.SetPasswordCmd{
		UserID: "u_1", Current: "not the password", New: "short",
	})
	var rejected *identity.PasswordRejected
	if !errors.As(err, &rejected) {
		t.Fatalf("err = %v, want the rule that was missed rather than a credential answer", err)
	}
	if errors.Is(err, identity.ErrCurrentPasswordIncorrect) {
		t.Error("a password that could never be accepted was answered as a wrong current password")
	}
}

// TestSetFirstPasswordNeedsOnlySession keeps an account an operator let in with
// a login code from being locked out of setting its first password.
func TestSetFirstPasswordNeedsOnlySession(t *testing.T) {
	svc := &identity.Service{
		Users:     &mock.MockUserStore{ByID: map[string]*coreidentity.User{"u_1": {ID: "u_1"}}},
		Passwords: &mock.MockPasswordStore{Hashes: map[string]string{}},
	}
	if err := svc.SetPassword(context.Background(), identity.SetPasswordCmd{
		UserID: "u_1", New: goodPassword,
	}); err != nil {
		t.Fatalf("an account with no password could not set one: %v", err)
	}
}

// TestRequestAccountRefusesSignupByDefault pins the default that keeps a
// reachable server from letting anyone claim a colleague's address. Nothing
// here verifies that whoever typed one controls it.
func TestRequestAccountRefusesSignupByDefault(t *testing.T) {
	svc := &identity.Service{Users: &mock.MockUserStore{}}

	_, err := svc.RequestAccount(context.Background(), "new@example.test", "", false, "free")
	if !errors.Is(err, identity.ErrSignupClosed) {
		t.Fatalf("err = %v, want ErrSignupClosed", err)
	}
	if !strings.Contains(err.Error(), "ask an administrator") {
		t.Errorf("the refusal does not say what to do instead: %q", err)
	}
}

// TestRequestAccountLoginIntentAnswersOnlyForAKnownAddress keeps the two
// intents apart: login says whether the address is registered, signup does not
// create one for an address that already is.
func TestRequestAccountLoginIntentAnswersOnlyForAKnownAddress(t *testing.T) {
	known := &coreidentity.User{ID: "u_1", Email: "known@example.test"}
	svc := &identity.Service{Users: &mock.MockUserStore{
		ByEmail: map[string]*coreidentity.User{known.Email: known},
		ByID:    map[string]*coreidentity.User{known.ID: known},
	}}

	outcome, err := svc.RequestAccount(context.Background(), known.Email, identity.IntentLogin, true, "free")
	if err != nil || outcome != identity.OutcomeAccountExists {
		t.Fatalf("outcome = %q, err = %v", outcome, err)
	}
	if _, err := svc.RequestAccount(context.Background(), "nobody@example.test", identity.IntentLogin, true, "free"); !errors.Is(err, identity.ErrAccountNotFound) {
		t.Fatalf("err = %v, want ErrAccountNotFound", err)
	}
	if _, err := svc.RequestAccount(context.Background(), known.Email, identity.IntentSignup, true, "free"); !errors.Is(err, identity.ErrEmailRegistered) {
		t.Fatalf("err = %v, want ErrEmailRegistered", err)
	}
}
