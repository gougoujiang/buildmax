package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
	"github.com/gougoujiang/buildmax/internal/infra/db"
	"github.com/gougoujiang/buildmax/internal/service/audit"
)

// The operator-side half of authentication. BuildMax has no way to email
// anything, so account creation, password setting, and code issuance happen
// here, on the machine that already holds the database credentials.
//
// A new account has no password. Either the operator sets one and passes it
// along, or they issue a login code and the person sets their own after signing
// in. The second is better — the password then exists only in the head of the
// person it belongs to — but the first is there for seeding a development
// database in one command.
//
// These commands read the same server.yaml the server does, so running them in
// a container or a pod needs no extra configuration.

// UserCommandUsage is the help text for `buildmax-server user`.
const UserCommandUsage = `Usage: buildmax-server user <command> [flags]

Commands:
  create <email>         Create an account and its personal team
  set-password <email>   Set an account's password, read from stdin
  login-code <email>     Issue a single-use login code for an existing account

Flags for login-code:
  --ttl duration         How long the code stays valid (default 1h)

set-password reads the password from stdin so it does not land in shell
history:

  echo -n 'correct horse battery staple' | buildmax-server user set-password alice@example.com

A login code is printed once and cannot be recovered. Deliver it to the person
yourself; BuildMax has no mail channel. It is also how someone who forgot their
password gets back in: sign in with the code, then set a new one.
See docs/deploy/authentication.md.
`

// RunUserCommand executes `buildmax-server user ...`. args excludes the "user"
// word itself.
func RunUserCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, UserCommandUsage)
		return errors.New("user: a command is required")
	}
	switch args[0] {
	case "create", "set-password", "login-code":
		store, closeStore, err := openUserStore(ctx)
		if err != nil {
			return err
		}
		defer closeStore()
		switch args[0] {
		case "create":
			return runUserCreate(ctx, args[1:], out, store)
		case "set-password":
			return runUserSetPassword(ctx, args[1:], out, os.Stdin, store)
		default:
			return runUserLoginCode(ctx, args[1:], out, store)
		}
	case "help", "-h", "--help":
		fmt.Fprint(out, UserCommandUsage)
		return nil
	default:
		fmt.Fprint(out, UserCommandUsage)
		return fmt.Errorf("user: unknown command %q", args[0])
	}
}

func runUserCreate(ctx context.Context, args []string, out io.Writer, store userAdminStore) error {
	email, err := emailArg("user create", args, out)
	if err != nil {
		return err
	}
	sc, err := config.LoadServerConfig()
	if err != nil {
		return fmt.Errorf("server config: %w", err)
	}
	user, err := store.CreateUser(ctx, email, sc.DefaultQuotaTier)
	if err != nil {
		if errors.Is(err, coreidentity.ErrEmailExists) {
			return fmt.Errorf("%s already has an account; issue a code with: buildmax-server user login-code %s", email, email)
		}
		return fmt.Errorf("create user: %w", err)
	}
	recordOperatorUserAudit(ctx, store, coreaudit.UserCreated, user.ID)
	fmt.Fprintf(out, "Created %s (%s) with a personal team. It has no password yet.\n\n", user.Email, user.ID)
	fmt.Fprintf(out, "Let them set their own:\n  buildmax-server user login-code %s\n\n", email)
	fmt.Fprintf(out, "Or set one now:\n  printf '%%s' '<password>' | buildmax-server user set-password %s\n", email)
	return nil
}

// runUserSetPassword sets an account's password, reading it from in.
//
// From stdin rather than a flag: a password on the command line is recorded in
// shell history and visible in the process list to everyone on the machine.
func runUserSetPassword(ctx context.Context, args []string, out io.Writer, in io.Reader, store userAdminStore) error {
	email, err := emailArg("user set-password", args, out)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(in, coreidentity.PasswordMaxLength+1))
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	// A trailing newline is what a pipe or a here-string adds, not part of what
	// anyone meant to type.
	password := strings.TrimRight(string(raw), "\r\n")
	if password == "" {
		return errors.New("no password on stdin; pipe one in, e.g. printf '%s' '<password>' | buildmax-server user set-password <email>")
	}
	if err := coreidentity.ValidatePassword(password); err != nil {
		return err
	}
	hash, err := coreidentity.HashPassword(password)
	if err != nil {
		return err
	}

	user, err := lookupUser(ctx, store, email)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("no account for %s; create one with: buildmax-server user create %s", email, email)
	}
	if err := store.SetPassword(ctx, user.ID, hash, time.Now().UTC()); err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	recordOperatorUserAudit(ctx, store, coreaudit.PasswordSet, user.ID)
	fmt.Fprintf(out, "Password set for %s.\n", user.Email)
	fmt.Fprintf(out, "Existing sessions are unaffected; revoke them separately if that is the intent.\n")
	return nil
}

func runUserLoginCode(ctx context.Context, args []string, out io.Writer, store userAdminStore) error {
	fs := flag.NewFlagSet("user login-code", flag.ContinueOnError)
	fs.SetOutput(out)
	ttl := fs.Duration("ttl", coreidentity.LoginCodeTTLDefault, "how long the code stays valid")
	if err := fs.Parse(args); err != nil {
		return err
	}
	email, err := requireEmailArg(fs.Args())
	if err != nil {
		return err
	}
	if *ttl <= 0 {
		return errors.New("--ttl must be positive")
	}
	user, err := lookupUser(ctx, store, email)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("no account for %s; create one with: buildmax-server user create %s", email, email)
	}
	code, expiresAt, err := store.CreateLoginCode(ctx, user.ID, *ttl)
	if err != nil {
		return fmt.Errorf("issue login code: %w", err)
	}
	recordOperatorUserAudit(ctx, store, coreaudit.LoginCodeIssued, user.ID)
	fmt.Fprintf(out, "Login code for %s:\n\n  %s\n\n", user.Email, code)
	fmt.Fprintf(out, "Valid until %s, and only once. It is not stored anywhere it can be read back,\n",
		expiresAt.Local().Format(time.RFC3339))
	fmt.Fprintf(out, "so a lost code means issuing another. Deliver it over a channel you trust.\n")
	return nil
}

// userAdminStore is the slice of the database the operator commands need.
type userAdminStore interface {
	coreidentity.UserStore
	coreidentity.LoginCodeStore
	coreidentity.PasswordStore
	coreaudit.Writer
}

// recordOperatorUserAudit writes an account action taken from the command line.
//
// These three commands wrote nothing to the trail until deployment
// administration was designed, which meant the most sensitive actions an
// operator takes — creating an account, setting its password, minting a way
// into it — were the only ones with no record. The actor is the system, for
// the same reason the model catalog commands say so: this runs from a shell on
// the machine that already holds the database credentials, and inventing a
// user id would put a name in the record that nothing verified.
func recordOperatorUserAudit(ctx context.Context, store coreaudit.Writer, action, userID string) {
	audit.NewRecorder(store).Record(ctx, coreaudit.Event{
		ActorType:  coreaudit.ActorSystem,
		ActorID:    coreaudit.ActorOperator,
		Action:     action,
		TargetType: "user",
		TargetID:   userID,
	})
}

// openUserStore connects using the same server.yaml the server reads, so an
// operator running this inside a container needs no extra configuration.
func openUserStore(ctx context.Context) (userAdminStore, func(), error) {
	sc, err := config.LoadServerConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("server config: %w", err)
	}
	dsn := sc.Database.DSN()
	if dsn == "" {
		return nil, nil, fmt.Errorf("database is not configured in %s", config.ServerConfigPath())
	}
	store, err := db.New(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	return store, func() { _ = store.Close() }, nil
}

func requireEmailArg(args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("an email address is required")
	}
	if len(args) > 1 {
		return "", fmt.Errorf("expected one email address, got %d arguments", len(args))
	}
	email := strings.TrimSpace(args[0])
	if email == "" || !strings.Contains(email, "@") {
		return "", fmt.Errorf("%q is not an email address", args[0])
	}
	return email, nil
}
