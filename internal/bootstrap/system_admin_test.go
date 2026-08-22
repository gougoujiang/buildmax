package bootstrap

import (
	"context"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
)

// fakeAdminStore satisfies adminStore from the in-memory mocks, so the command
// logic is exercised without a database. The commands' own store opener is what
// needs one, and it is the part with no logic in it.
type fakeAdminStore struct {
	*mock.MockUserStore
	*mock.MockSystemGrantStore
	*mock.MockAuditStore
}

func newAdminFixture(t *testing.T) (*fakeAdminStore, *model.User) {
	t.Helper()
	users := &mock.MockUserStore{}
	user, err := users.CreateUser(context.Background(), "alice@example.com", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return &fakeAdminStore{
		MockUserStore:        users,
		MockSystemGrantStore: &mock.MockSystemGrantStore{},
		MockAuditStore:       &mock.MockAuditStore{},
	}, user
}

func actions(store *fakeAdminStore) []string {
	out := make([]string, 0, len(store.MockAuditStore.Events))
	for _, e := range store.MockAuditStore.Events {
		out = append(out, e.Action)
	}
	return out
}

// TestAdminGrantRequiresAnExistingAccount pins the decision that granting does
// not create an account. Creating one and minting deployment authority are two
// decisions, and a command that silently did both would make the second
// invisible.
func TestAdminGrantRequiresAnExistingAccount(t *testing.T) {
	store, _ := newAdminFixture(t)
	var out strings.Builder

	err := runAdminGrant(context.Background(), []string{"nobody@example.com"}, &out, store)
	if err == nil {
		t.Fatal("granting to an unknown address should fail")
	}
	if !strings.Contains(err.Error(), "buildmax-server user create") {
		t.Errorf("error should name the command that fixes it, got %q", err)
	}
	if len(store.MockSystemGrantStore.Grants) != 0 {
		t.Errorf("no grant should exist: %+v", store.MockSystemGrantStore.Grants)
	}
	if len(store.MockAuditStore.Events) != 0 {
		t.Errorf("a refused grant is not an audit event here: %v", actions(store))
	}
}

func TestAdminGrantAndRevoke(t *testing.T) {
	store, user := newAdminFixture(t)
	ctx := context.Background()

	var granted strings.Builder
	if err := runAdminGrant(ctx, []string{user.Email}, &granted, store); err != nil {
		t.Fatalf("runAdminGrant: %v", err)
	}
	roles, err := store.ActiveSystemRoles(ctx, user.ID)
	if err != nil || len(roles) != 1 || roles[0] != model.SystemRoleAdmin {
		t.Fatalf("ActiveSystemRoles = %v, %v; want [%s]", roles, err, model.SystemRoleAdmin)
	}
	// An account with no password cannot use the authority it was just given,
	// and the operator should not have to work that out for themselves.
	if !strings.Contains(granted.String(), "buildmax-server user login-code") {
		t.Errorf("grant output should point at the way in for a passwordless account, got:\n%s", granted.String())
	}

	// The grant is attributed to the binary, not to an invented user, and the
	// grant row and the audit event agree on the name.
	if got := store.MockSystemGrantStore.Grants[0].GrantedBy; got != model.AuditActorOperator {
		t.Errorf("GrantedBy = %q, want %q", got, model.AuditActorOperator)
	}
	event := store.MockAuditStore.Events[0]
	if event.Action != model.AuditSystemAdminGranted ||
		event.ActorType != model.AuditActorSystem ||
		event.ActorID != model.AuditActorOperator ||
		event.TargetID != user.ID ||
		event.TeamID != "" {
		t.Errorf("grant event wrong: %+v", event)
	}

	// Granting twice is refused rather than creating a second row.
	var again strings.Builder
	if err := runAdminGrant(ctx, []string{user.Email}, &again, store); err == nil ||
		!strings.Contains(err.Error(), "already holds") {
		t.Errorf("second grant err = %v, want an 'already holds' message", err)
	}

	var revoked strings.Builder
	if err := runAdminRevoke(ctx, []string{user.Email}, &revoked, store); err != nil {
		t.Fatalf("runAdminRevoke: %v", err)
	}
	roles, err = store.ActiveSystemRoles(ctx, user.ID)
	if err != nil || len(roles) != 0 {
		t.Fatalf("after revoke ActiveSystemRoles = %v, %v; want empty", roles, err)
	}
	if want := []string{model.AuditSystemAdminGranted, model.AuditSystemAdminRevoked}; len(actions(store)) != 2 ||
		actions(store)[0] != want[0] || actions(store)[1] != want[1] {
		t.Errorf("audit actions = %v, want %v", actions(store), want)
	}
	// Revocation does not end their sessions, and saying so is the difference
	// between an operator who follows up and one who thinks they are done.
	if !strings.Contains(revoked.String(), "sessions are untouched") {
		t.Errorf("revoke output should say what it did not do, got:\n%s", revoked.String())
	}
}

// TestAdminRevokeLastGrantWarns covers the asymmetry in section 6 of the
// design: the API refuses to revoke the last grant and this command allows it,
// because this command is the recovery path. Allowing it silently would be the
// bug.
func TestAdminRevokeLastGrantWarns(t *testing.T) {
	store, user := newAdminFixture(t)
	ctx := context.Background()
	if err := runAdminGrant(ctx, []string{user.Email}, &strings.Builder{}, store); err != nil {
		t.Fatalf("runAdminGrant: %v", err)
	}

	var out strings.Builder
	if err := runAdminRevoke(ctx, []string{user.Email}, &out, store); err != nil {
		t.Fatalf("runAdminRevoke: %v", err)
	}
	if !strings.Contains(out.String(), "no "+model.SystemRoleAdmin) {
		t.Errorf("revoking the last grant should say the deployment now has none, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "buildmax-server admin grant") {
		t.Errorf("the warning should name the way back, got:\n%s", out.String())
	}
}

// TestAdminRevokeIsIdempotent: revoking an authority nobody holds is the end
// state that was asked for, not an error.
func TestAdminRevokeIsIdempotent(t *testing.T) {
	store, user := newAdminFixture(t)
	var out strings.Builder

	if err := runAdminRevoke(context.Background(), []string{user.Email}, &out, store); err != nil {
		t.Fatalf("revoking an absent grant should succeed, got %v", err)
	}
	if !strings.Contains(out.String(), "nothing to revoke") {
		t.Errorf("output should say nothing happened, got:\n%s", out.String())
	}
	if len(store.MockAuditStore.Events) != 0 {
		t.Errorf("nothing happened, so nothing is recorded: %v", actions(store))
	}
}

func TestAdminListShowsHoldersAndHistory(t *testing.T) {
	store, user := newAdminFixture(t)
	ctx := context.Background()

	var empty strings.Builder
	if err := runAdminList(ctx, nil, &empty, store); err != nil {
		t.Fatalf("runAdminList: %v", err)
	}
	if !strings.Contains(empty.String(), "No account holds a system role") {
		t.Errorf("empty list should say so, got:\n%s", empty.String())
	}

	if err := runAdminGrant(ctx, []string{user.Email}, &strings.Builder{}, store); err != nil {
		t.Fatalf("runAdminGrant: %v", err)
	}
	var listed strings.Builder
	if err := runAdminList(ctx, nil, &listed, store); err != nil {
		t.Fatalf("runAdminList: %v", err)
	}
	if !strings.Contains(listed.String(), user.Email) || !strings.Contains(listed.String(), "active") {
		t.Errorf("list should name the holder and its state, got:\n%s", listed.String())
	}

	if err := runAdminRevoke(ctx, []string{user.Email}, &strings.Builder{}, store); err != nil {
		t.Fatalf("runAdminRevoke: %v", err)
	}
	var afterRevoke strings.Builder
	if err := runAdminList(ctx, nil, &afterRevoke, store); err != nil {
		t.Fatalf("runAdminList: %v", err)
	}
	if strings.Contains(afterRevoke.String(), user.Email) {
		t.Errorf("default list should show only live grants, got:\n%s", afterRevoke.String())
	}

	// --all is how the history is read: the row stays, so who held authority
	// and when it ended is still answerable.
	var all strings.Builder
	if err := runAdminList(ctx, []string{"--all"}, &all, store); err != nil {
		t.Fatalf("runAdminList --all: %v", err)
	}
	if !strings.Contains(all.String(), user.Email) || !strings.Contains(all.String(), "revoked") {
		t.Errorf("--all should show the retired grant, got:\n%s", all.String())
	}
}

// TestAdminCommandsRejectBadArguments keeps the argument handling matching the
// sibling `user` commands, which a copied flag set makes easy to get wrong.
func TestAdminCommandsRejectBadArguments(t *testing.T) {
	store, _ := newAdminFixture(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"no email", nil},
		{"two emails", []string{"a@example.com", "b@example.com"}},
		{"not an email", []string{"alice"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runAdminGrant(ctx, tc.args, &strings.Builder{}, store); err == nil {
				t.Errorf("grant %v should fail", tc.args)
			}
			if err := runAdminRevoke(ctx, tc.args, &strings.Builder{}, store); err == nil {
				t.Errorf("revoke %v should fail", tc.args)
			}
		})
	}
}

// TestAdminCommandUsageIsReachable: `admin` with no arguments and `admin help`
// both print the usage, and only the first is an error.
func TestAdminCommandUsageIsReachable(t *testing.T) {
	var none strings.Builder
	if err := RunAdminCommand(context.Background(), nil, &none); err == nil {
		t.Error("no subcommand should be an error")
	}
	if !strings.Contains(none.String(), "buildmax-server admin") {
		t.Errorf("usage should be printed, got:\n%s", none.String())
	}

	var help strings.Builder
	if err := RunAdminCommand(context.Background(), []string{"help"}, &help); err != nil {
		t.Errorf("help should not be an error, got %v", err)
	}
	if !strings.Contains(help.String(), "Revoking the last grant") {
		t.Errorf("help should explain the recovery asymmetry, got:\n%s", help.String())
	}
}
