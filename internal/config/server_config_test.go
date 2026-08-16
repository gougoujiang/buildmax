package config

import (
	"strings"
	"testing"
)

// TestServerDBConfig_DSNUsesTLS covers the connection a deployment makes to a
// database it already runs. Most managed MySQL either requires TLS or is
// reached across a network where the credentials must not travel in the clear,
// and until this existed the DSN had no way to ask for it.
func TestServerDBConfig_DSNUsesTLS(t *testing.T) {
	base := ServerDBConfig{Host: "db.example", Port: 3306, User: "u", Password: "p", Name: "buildmax"}

	t.Run("defaults to preferred", func(t *testing.T) {
		// preferred upgrades whenever the server offers TLS and behaves as
		// before against one that does not, so it introduces no failure mode a
		// plaintext connection did not already have.
		if got := base.DSN(); !strings.Contains(got, "tls="+DefaultDBTLSMode) {
			t.Errorf("DSN = %q, want tls=%s", got, DefaultDBTLSMode)
		}
	})

	t.Run("an explicit mode is passed through", func(t *testing.T) {
		for _, mode := range []string{"true", "skip-verify", "false"} {
			cfg := base
			cfg.TLS = mode
			if got := cfg.DSN(); !strings.Contains(got, "tls="+mode) {
				t.Errorf("DSN for %q = %q", mode, got)
			}
		}
	})

	t.Run("keeps the parameters the driver already needed", func(t *testing.T) {
		got := base.DSN()
		for _, want := range []string{"charset=utf8mb4", "parseTime=True"} {
			if !strings.Contains(got, want) {
				t.Errorf("DSN = %q, missing %q", got, want)
			}
		}
	})
}
