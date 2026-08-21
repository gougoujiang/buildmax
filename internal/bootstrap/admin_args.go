package bootstrap

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// emailArg parses a subcommand's flags and returns its single email argument.
func emailArg(name string, args []string, out io.Writer) (string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	return requireEmailArg(fs.Args())
}

// lookupUser resolves an account, reporting a missing one as (nil, nil).
//
// The caller writes its own not-found message: grant points the operator at
// `user create`, revoke does not, because suggesting an account be created is
// the wrong advice when the command's job is to take access away.
func lookupUser(ctx context.Context, users model.UserStore, email string) (*model.User, error) {
	user, err := users.UserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("look up user: %w", err)
	}
	return user, nil
}
