package bootstrap

import (
	"context"
	"strings"
	"testing"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	"github.com/gougoujiang/buildmax/internal/mock"
)

// fakeUserAdminStore satisfies userAdminStore from the in-memory mocks.
type fakeUserAdminStore struct {
	*mock.MockUserStore
	*mock.MockLoginCodeStore
	*mock.MockPasswordStore
	*mock.MockAuditStore
}

func newUserAdminFixture() *fakeUserAdminStore {
	return &fakeUserAdminStore{
		MockUserStore:      &mock.MockUserStore{},
		MockLoginCodeStore: &mock.MockLoginCodeStore{},
		MockPasswordStore:  &mock.MockPasswordStore{},
		MockAuditStore:     &mock.MockAuditStore{},
	}
}

// TestOperatorAccountCommandsAreRecorded is the assertion behind the claim that
// the operator commands stopped being invisible.
//
// Creating an account, setting its password, and minting a way into it are the
// most sensitive actions an operator takes, and until deployment administration
// was designed all three wrote nothing to the trail. The model catalog commands
// always did, so this was an inconsistency rather than a policy — and a policy
// is what a test makes it.
func TestOperatorAccountCommandsAreRecorded(t *testing.T) {
	store := newUserAdminFixture()
	ctx := context.Background()
	email := "alice@example.com"

	if err := runUserCreate(ctx, []string{email}, &strings.Builder{}, store); err != nil {
		t.Fatalf("runUserCreate: %v", err)
	}
	user, err := store.UserByEmail(ctx, email)
	if err != nil || user == nil {
		t.Fatalf("the account was not created: %+v, %v", user, err)
	}

	if err := runUserSetPassword(ctx, []string{email}, &strings.Builder{},
		strings.NewReader("correct horse battery staple\n"), store); err != nil {
		t.Fatalf("runUserSetPassword: %v", err)
	}
	if err := runUserLoginCode(ctx, []string{email}, &strings.Builder{}, store); err != nil {
		t.Fatalf("runUserLoginCode: %v", err)
	}

	want := []string{coreaudit.UserCreated, coreaudit.PasswordSet, coreaudit.LoginCodeIssued}
	events := store.MockAuditStore.Events
	if len(events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(events), len(want), events)
	}
	for i, action := range want {
		e := events[i]
		if e.Action != action {
			t.Errorf("event %d action = %q, want %q", i, e.Action, action)
		}
		// The actor is the binary, not an invented user id. A command runs on
		// the machine that holds the database credentials and has no session to
		// name; naming the machine is less than naming a person and more than
		// recording nothing.
		if e.ActorType != coreaudit.ActorSystem || e.ActorID != coreaudit.ActorOperator {
			t.Errorf("event %d should name the operator binary: %+v", i, e)
		}
		if e.TargetType != "user" || e.TargetID != user.ID {
			t.Errorf("event %d should name the account: %+v", i, e)
		}
		// The trail says a thing happened, never what it became: no password,
		// no code.
		if strings.Contains(e.Detail, "horse") || strings.Contains(e.Detail, "mock-code") {
			t.Errorf("event %d carried a credential in Detail: %+v", i, e)
		}
	}
}

// TestOperatorCommandsRecordNothingWhenTheyFail: a command that refused is not
// a command that happened.
func TestOperatorCommandsRecordNothingWhenTheyFail(t *testing.T) {
	store := newUserAdminFixture()
	ctx := context.Background()

	if err := runUserLoginCode(ctx, []string{"nobody@example.com"}, &strings.Builder{}, store); err == nil {
		t.Fatal("issuing a code for an unknown account should fail")
	}
	if err := runUserSetPassword(ctx, []string{"nobody@example.com"}, &strings.Builder{},
		strings.NewReader("correct horse battery staple"), store); err == nil {
		t.Fatal("setting a password on an unknown account should fail")
	}
	if len(store.MockAuditStore.Events) != 0 {
		t.Errorf("refused commands were recorded: %+v", store.MockAuditStore.Events)
	}
}

// TestUserCreateRefusesADuplicate keeps the message that tells an operator what
// to do instead, which is the whole value of the error.
func TestUserCreateRefusesADuplicate(t *testing.T) {
	store := newUserAdminFixture()
	ctx := context.Background()
	email := "alice@example.com"

	if err := runUserCreate(ctx, []string{email}, &strings.Builder{}, store); err != nil {
		t.Fatalf("runUserCreate: %v", err)
	}
	err := runUserCreate(ctx, []string{email}, &strings.Builder{}, store)
	if err == nil {
		t.Fatal("creating the same account twice should fail")
	}
	if !strings.Contains(err.Error(), "buildmax-server user login-code") {
		t.Errorf("the error should name the command that fixes it, got %q", err)
	}
	if len(store.MockAuditStore.Events) != 1 {
		t.Errorf("the refused second create was recorded: %+v", store.MockAuditStore.Events)
	}
}
