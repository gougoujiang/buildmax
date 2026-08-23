package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/interface/auth"
)

// resolveModelSource fetches what this session's models are, turning an expired
// login into an answerable question rather than a bare failure.
//
// The session does not run on an expired login, and it does not quietly fall
// back to local models either: that would send a prompt to a provider the user
// did not choose for this session. What it does is name the one action that
// returns the client to local mode — `buildmax logout`, which removes the
// credentials that are the mode. See docs/design/client-modes.md section 8.
func resolveModelSource(ctx context.Context) (auth.ModelSource, error) {
	source, err := auth.ResolveModelSource(ctx)
	if err == nil {
		return source, nil
	}
	if errors.Is(err, auth.ErrLoginExpired) {
		return auth.ModelSource{}, fmt.Errorf("%w\n\n"+
			"Sign in again with `buildmax login`, or run `buildmax logout` to use the\n"+
			"models in settings.yaml. Nothing here runs until one of those happens: a\n"+
			"session must not send prompts somewhere you did not choose", err)
	}
	return auth.ModelSource{}, err
}
