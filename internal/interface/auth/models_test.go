package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/infra/llmwire"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

// deploymentOffering is a server that answers the model listing with what the
// test staged, recording the path it was asked for.
func deploymentOffering(t *testing.T, models []llmwire.Model) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != llmwire.ModelsPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmwire.ModelsResponse{Models: models})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestResolveModelSourceWithoutALoginIsLocal is the mode rule: no credentials
// means the caller uses settings.yaml, and nothing is fetched.
func TestResolveModelSourceWithoutALoginIsLocal(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxHome, t.TempDir())

	source, err := ResolveModelSource(context.Background())
	if err != nil {
		t.Fatalf("ResolveModelSource: %v", err)
	}
	if source.Managed() {
		t.Errorf("source = %+v, want local mode", source)
	}
	if len(source.Entries) != 0 {
		t.Errorf("Entries = %+v, want none in local mode", source.Entries)
	}
}

// TestResolveModelSourceFetchesWhatTheDeploymentOffers is managed mode: the
// models are the server's, and each carries the window a session compacts
// against but no endpoint or credential of its own.
func TestResolveModelSourceFetchesWhatTheDeploymentOffers(t *testing.T) {
	srv := deploymentOffering(t, []llmwire.Model{
		{Name: "Fast", ContextWindow: 128_000},
		{Name: "Deep", ContextWindow: 200_000, Vision: true, Default: true},
	})
	t.Setenv(config.EnvKeyBuildmaxHome, t.TempDir())
	saveUsableLogin(t, srv.URL)

	source, err := ResolveModelSource(context.Background())
	if err != nil {
		t.Fatalf("ResolveModelSource: %v", err)
	}
	if !source.Managed() || source.ServerURL != srv.URL {
		t.Fatalf("source = %+v, want managed mode against %s", source, srv.URL)
	}
	if len(source.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2", len(source.Entries))
	}
	if source.Default != "Deep" {
		t.Errorf("Default = %q, want the model the server marked", source.Default)
	}

	fast := source.Entries[0]
	if fast.Name != "Fast" || fast.Model != "Fast" {
		t.Errorf("entry = %+v, want the catalog name as both identity and model", fast)
	}
	if fast.ContextWindow != 128_000 {
		t.Errorf("ContextWindow = %d, want the server's", fast.ContextWindow)
	}
	if fast.APIKey != "" || fast.APIURL != "" {
		t.Errorf("entry = %+v, want no credential and no endpoint", fast)
	}
	if !source.Entries[1].Vision {
		t.Error("vision was dropped, so an image would never be sent to a model that reads one")
	}
}

// TestResolveModelSourceReportsAnExpiredLogin is the no-fallback rule at its
// sharpest.
//
// A spent login is not "signed out": auth.Info reports it that way, which is
// right for a command asking whether to offer an account, and using that here
// would silently put the session in local mode — sending the next prompt to a
// provider nobody chose for it. What decides the mode is the credentials file
// existing, so this must report the expiry and let the user answer it.
func TestResolveModelSourceReportsAnExpiredLogin(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxHome, t.TempDir())
	saveExpiredLogin(t, "https://buildmax.example.com")

	// The premise: Info reports this login as signed out.
	info, err := Info()
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.LoggedIn {
		t.Fatal("the fixture is not an expired login")
	}

	source, err := ResolveModelSource(context.Background())
	if !errors.Is(err, ErrLoginExpired) {
		t.Fatalf("want ErrLoginExpired, got %v", err)
	}
	if source.Managed() || len(source.Entries) > 0 {
		t.Errorf("source = %+v, want nothing usable alongside the error", source)
	}
}

// An empty catalog is reported rather than returned as a usable managed mode
// with nothing in it, which would fail later with no explanation.
func TestResolveModelSourceRejectsAnEmptyCatalog(t *testing.T) {
	srv := deploymentOffering(t, nil)
	t.Setenv(config.EnvKeyBuildmaxHome, t.TempDir())
	saveUsableLogin(t, srv.URL)

	if _, err := ResolveModelSource(context.Background()); err == nil {
		t.Fatal("an empty catalog produced a usable managed mode")
	}
}

func saveUsableLogin(t *testing.T, serverURL string) {
	t.Helper()
	creds := &Credentials{
		ServerURL: serverURL,
		Token:     testsupport.SignJWTWithExp("u_1", "secret", 24*time.Hour),
		UserID:    "u_1",
		Email:     "someone@example.com",
	}
	if err := Save(creds, config.AuthPath()); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// An expired access token and no refresh token: the login is stored, and it can
// no longer authenticate anything.
func saveExpiredLogin(t *testing.T, serverURL string) {
	t.Helper()
	creds := &Credentials{
		ServerURL: serverURL,
		Token:     testsupport.SignJWTWithExp("u_1", "secret", -time.Hour),
		UserID:    "u_1",
	}
	if err := Save(creds, config.AuthPath()); err != nil {
		t.Fatalf("Save: %v", err)
	}
}
