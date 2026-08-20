package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

// storeLogin writes credentials into an isolated BUILDMAX_HOME.
func storeLogin(t *testing.T, serverURL, token string) {
	t.Helper()
	t.Setenv(config.EnvKeyBuildmaxHome, t.TempDir())
	if serverURL == "" && token == "" {
		return
	}
	if err := Save(&Credentials{ServerURL: serverURL, Token: token, UserID: "u_1"}, config.AuthPath()); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func TestTokenForServerReturnsTheStoredToken(t *testing.T) {
	token := testsupport.SignJWTWithExp("u_1", "secret", 24*time.Hour)
	storeLogin(t, "https://buildmax.example.com", token)

	got, err := TokenForServer("https://buildmax.example.com")
	if err != nil {
		t.Fatalf("TokenForServer: %v", err)
	}
	if got != token {
		t.Error("TokenForServer returned a different token")
	}

	// A trailing slash is the same server, not a different one.
	if _, err := TokenForServer("https://buildmax.example.com/"); err != nil {
		t.Errorf("a trailing slash was treated as a different server: %v", err)
	}
}

// TestTokenForServerRefusesAnotherHost is the guard that matters most here:
// settings.yaml is writable by anything running as the user, so a managed entry
// naming a different server must not be handed the BuildMax credential.
func TestTokenForServerRefusesAnotherHost(t *testing.T) {
	token := testsupport.SignJWTWithExp("u_1", "secret", 24*time.Hour)
	storeLogin(t, "https://buildmax.example.com", token)

	got, err := TokenForServer("https://attacker.example.net")
	if err == nil {
		t.Fatal("the token was handed to a different server")
	}
	if got != "" {
		t.Error("a refused lookup still returned a token")
	}
	if !strings.Contains(err.Error(), "attacker.example.net") {
		t.Errorf("the error does not name the mismatched server: %v", err)
	}
}

func TestTokenForServerRequiresAUsableLogin(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		token     string
		ask       string
		wantIn    string
	}{
		{
			name:   "entry has no server_url",
			ask:    "",
			wantIn: "no server_url",
		},
		{
			name:   "not logged in",
			ask:    "https://buildmax.example.com",
			wantIn: "not logged in",
		},
		{
			name:      "login expired",
			serverURL: "https://buildmax.example.com",
			token:     testsupport.SignJWTWithExp("u_1", "secret", -time.Hour),
			ask:       "https://buildmax.example.com",
			wantIn:    "expired",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			storeLogin(t, tc.serverURL, tc.token)
			if _, err := TokenForServer(tc.ask); err == nil || !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("want an error mentioning %q, got %v", tc.wantIn, err)
			}
		})
	}
}
