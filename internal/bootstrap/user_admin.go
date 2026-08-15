package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/db"
)

// The operator-side half of authentication. BuildMax has no way to email a
// code, so account creation and code issuance happen here, on the machine that
// already holds the database credentials, and the operator delivers the code
// over whatever channel they already use.
//
// These commands read the same server.yaml the server does, so running them in
// a container or a pod needs no extra configuration.

// UserCommandUsage is the help text for `buildmax-server user`.
const UserCommandUsage = `Usage: buildmax-server user <command> [flags]

Commands:
  create <email>       Create an account and its personal team
  login-code <email>   Issue a single-use login code for an existing account

Flags for login-code:
  --ttl duration       How long the code stays valid (default 1h)

The code is printed once and cannot be recovered. Deliver it to the person
yourself; BuildMax has no mail channel. See docs/deploy/authentication.md.
`

// RunUserCommand executes `buildmax-server user ...`. args excludes the "user"
// word itself.
func RunUserCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, UserCommandUsage)
		return errors.New("user: a command is required")
	}
	switch args[0] {
	case "create":
		return runUserCreate(ctx, args[1:], out)
	case "login-code":
		return runUserLoginCode(ctx, args[1:], out)
	case "help", "-h", "--help":
		fmt.Fprint(out, UserCommandUsage)
		return nil
	default:
		fmt.Fprint(out, UserCommandUsage)
		return fmt.Errorf("user: unknown command %q", args[0])
	}
}

func runUserCreate(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("user create", flag.ContinueOnError)
	fs.SetOutput(out)
	if err := fs.Parse(args); err != nil {
		return err
	}
	email, err := requireEmailArg(fs.Args())
	if err != nil {
		return err
	}
	store, closeStore, err := openUserStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	sc, err := config.LoadServerConfig()
	if err != nil {
		return fmt.Errorf("server config: %w", err)
	}
	user, err := store.CreateUser(ctx, email, sc.DefaultQuotaTier)
	if err != nil {
		if errors.Is(err, model.ErrEmailExists) {
			return fmt.Errorf("%s already has an account; issue a code with: buildmax-server user login-code %s", email, email)
		}
		return fmt.Errorf("create user: %w", err)
	}
	fmt.Fprintf(out, "Created %s (%s) with a personal team.\n\n", user.Email, user.UserID)
	fmt.Fprintf(out, "Next, issue a login code:\n  buildmax-server user login-code %s\n", email)
	return nil
}

func runUserLoginCode(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("user login-code", flag.ContinueOnError)
	fs.SetOutput(out)
	ttl := fs.Duration("ttl", model.LoginCodeTTLDefault, "how long the code stays valid")
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
	store, closeStore, err := openUserStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	user, err := store.UserByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("look up user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("no account for %s; create one with: buildmax-server user create %s", email, email)
	}
	code, expiresAt, err := store.CreateLoginCode(ctx, user.UserID, *ttl)
	if err != nil {
		return fmt.Errorf("issue login code: %w", err)
	}
	fmt.Fprintf(out, "Login code for %s:\n\n  %s\n\n", user.Email, code)
	fmt.Fprintf(out, "Valid until %s, and only once. It is not stored anywhere it can be read back,\n",
		time.Unix(expiresAt, 0).Format(time.RFC3339))
	fmt.Fprintf(out, "so a lost code means issuing another. Deliver it over a channel you trust.\n")
	return nil
}

// userAdminStore is the slice of the database the operator commands need.
type userAdminStore interface {
	model.UserStore
	model.LoginCodeStore
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
