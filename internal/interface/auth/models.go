package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/interface/client"
)

// ErrLoginExpired means a login is stored but no longer works: its refresh
// token expired, or the session was revoked.
//
// It is distinct from a deployment that cannot be reached, because the answer
// is different. An unreachable server may come back; an expired login will not,
// and the session cannot continue until someone signs in again or returns to
// local mode. Neither is a reason to quietly use local models: that would send
// a prompt somewhere the user did not choose. See
// docs/design/client-modes.md section 8.
var ErrLoginExpired = errors.New("login has expired")

// ModelSource is where a surface's models come from.
//
// A login is the mode: with one the models are the deployment's and every
// prompt goes there, without one they are the ones in settings.yaml and each
// call is made from this machine. The two are never mixed — see
// docs/design/client-modes.md section 1.
type ModelSource struct {
	// ServerURL is the deployment serving these models, empty in local mode.
	ServerURL string
	// Entries is what that deployment offers. Nil in local mode, where the
	// caller uses settings.yaml instead.
	Entries []config.ModelEntry
	// Default is the model name a new session starts with, as the deployment
	// marked it. Empty in local mode, where settings.yaml says.
	Default string
}

// Managed reports whether these models are served by a deployment.
func (s ModelSource) Managed() bool { return s.ServerURL != "" }

// StoredLogin returns the credentials on disk whether or not they still work,
// and nil when there are none.
//
// This is the mode, and it is deliberately not Info(): Info answers "am I
// signed in" and reports a spent login as signed out, which would make an
// expired session look like local mode. See ResolveModelSource.
func StoredLogin() (*Credentials, error) {
	creds, err := Load(config.AuthPath())
	if err != nil {
		return nil, err
	}
	if creds == nil || creds.Token == "" || creds.ServerURL == "" {
		return nil, nil
	}
	return creds, nil
}

// ResolveModelSource asks the deployment this machine is signed in to what it
// offers, or reports local mode when there is no login.
//
// Managed mode fetches eagerly rather than on first use. The list carries each
// model's context window, which the session needs before it can compact, and a
// deployment that cannot be reached must fail where the user is still deciding
// what to do rather than mid-turn. There is no fallback to local models: that
// would send a prompt somewhere the user did not choose.
func ResolveModelSource(ctx context.Context) (ModelSource, error) {
	// Stored credentials decide the mode, not whether they still work. Info()
	// answers "am I signed in" and reports a spent login as signed out, which is
	// right for a command asking whether to offer an account — but using it here
	// would turn an expired session into local mode on its own, sending the next
	// prompt to a provider nobody chose for it.
	creds, err := StoredLogin()
	if err != nil {
		return ModelSource{}, fmt.Errorf("load auth: %w", err)
	}
	if creds == nil {
		return ModelSource{}, nil
	}
	serverURL := creds.ServerURL

	token, err := TokenForServer(serverURL)
	if err != nil {
		return ModelSource{}, fmt.Errorf("%w: signed in to %s, but the credential no longer works (%v)",
			ErrLoginExpired, serverURL, err)
	}
	models, err := client.NewClient(serverURL).ListServerModels(ctx, token)
	if err != nil {
		return ModelSource{}, fmt.Errorf("list the models %s offers: %w", serverURL, err)
	}
	if len(models) == 0 {
		return ModelSource{}, fmt.Errorf("%s offers no models: its catalog is empty", serverURL)
	}

	source := ModelSource{ServerURL: serverURL, Entries: make([]config.ModelEntry, 0, len(models))}
	for _, m := range models {
		// No api_url and no api_key: the deployment holds both. The name is
		// what a completion request names, so it is both the entry's identity
		// and its model field.
		source.Entries = append(source.Entries, config.ModelEntry{
			Model:         m.Name,
			Name:          m.Name,
			ContextWindow: m.ContextWindow,
			Vision:        m.Vision,
		})
		if m.Default {
			source.Default = m.Name
		}
	}
	return source, nil
}
