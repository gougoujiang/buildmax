package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

// writeServerYAML puts a server.yaml in a temp BUILDMAX_HOME and points config at it.
func writeServerYAML(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, "server.yaml"), []byte(body), 0o600); err != nil {
			t.Fatalf("write server.yaml: %v", err)
		}
	}
	t.Setenv(config.EnvKeyBuildmaxHome, dir)
}

// TestServerConfigSecretEnvOverrides is the contract the Kubernetes deployment
// depends on: non-secret values come from server.yaml, credentials come from the
// environment, and the environment wins.
func TestServerConfigSecretEnvOverrides(t *testing.T) {
	writeServerYAML(t, `
jwt_secret: from-file
database:
  host: mysql.internal
  password: from-file
storage:
  minio:
    access_key: from-file
    secret_key: from-file
conversation:
  model:
    api_key: from-file
`)
	t.Setenv(config.EnvKeyBuildmaxDatabasePassword, "from-env")
	t.Setenv(config.EnvKeyBuildmaxMinIOAccessKey, "from-env")
	t.Setenv(config.EnvKeyBuildmaxMinIOSecretKey, "from-env")
	t.Setenv(config.EnvKeyBuildmaxConversationAPIKey, "from-env")

	cfg, err := config.LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig: %v", err)
	}

	cases := map[string]string{
		"database.password":          cfg.Database.Password,
		"storage.minio.access_key":   cfg.Storage.MinIO.AccessKey,
		"storage.minio.secret_key":   cfg.Storage.MinIO.SecretKey,
		"conversation.model.api_key": cfg.Conversation.Model.APIKey,
	}
	for field, got := range cases {
		if got != "from-env" {
			t.Errorf("%s = %q, want the environment override %q", field, got, "from-env")
		}
	}

	// Non-secret values must still come from the file, and unset overrides must
	// not blank out a value the file provides.
	if cfg.Database.Host != "mysql.internal" {
		t.Errorf("database.host = %q, want the value from server.yaml", cfg.Database.Host)
	}
	if cfg.JWTSecret != "from-file" {
		t.Errorf("jwt_secret = %q, want the value from server.yaml when the env var is unset", cfg.JWTSecret)
	}
}

// TestServerConfigCORSOriginEnvOverride covers the one override that carries no
// secret. The Compose stack publishes the Portal on a port chosen in .env and
// derives this from it, so a file value it disagrees with must lose — otherwise
// moving that port is two edits and the browser blocks every request after the
// first one.
func TestServerConfigCORSOriginEnvOverride(t *testing.T) {
	writeServerYAML(t, "cors_origin: http://localhost:8080\n")
	t.Setenv(config.EnvKeyBuildmaxCORSOrigin, "http://localhost:8081")

	cfg, err := config.LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig: %v", err)
	}
	if cfg.CORSOrigin != "http://localhost:8081" {
		t.Errorf("cors_origin = %q, want the environment override", cfg.CORSOrigin)
	}
}

// An unset override must leave a file value alone, the same way the credential
// overrides do.
func TestServerConfigCORSOriginFromFile(t *testing.T) {
	writeServerYAML(t, "cors_origin: http://localhost:8080\n")

	cfg, err := config.LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig: %v", err)
	}
	if cfg.CORSOrigin != "http://localhost:8080" {
		t.Errorf("cors_origin = %q, want the value from server.yaml", cfg.CORSOrigin)
	}
}

// TestServerConfigEnvOnly covers the container case: no server.yaml on disk yet,
// credentials supplied entirely by the environment.
func TestServerConfigEnvOnly(t *testing.T) {
	writeServerYAML(t, "")
	t.Setenv(config.EnvKeyBuildmaxJWTSecret, "secret-from-env")

	cfg, err := config.LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig: %v", err)
	}
	if cfg.JWTSecret != "secret-from-env" {
		t.Errorf("jwt_secret = %q, want %q", cfg.JWTSecret, "secret-from-env")
	}
	if cfg.Worker.Binary != "buildmax-worker" {
		t.Errorf("worker.binary = %q, want the default to survive", cfg.Worker.Binary)
	}
}

// A server that never mentions allow_signup must be closed. The default lives in
// LoadServerConfig by omission, which is exactly the kind of thing a later
// "let's set a default for everything" pass would flip without noticing.
func TestServerConfigSignupIsClosedByDefault(t *testing.T) {
	writeServerYAML(t, "port: 5678\n")
	sc, err := config.LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig: %v", err)
	}
	if sc.AllowSignup {
		t.Error("allow_signup defaults to true; self-registration must be opt-in")
	}
}

func TestServerConfigSignupCanBeOpened(t *testing.T) {
	writeServerYAML(t, "allow_signup: true\n")
	sc, err := config.LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig: %v", err)
	}
	if !sc.AllowSignup {
		t.Error("allow_signup: true did not bind")
	}
}

func TestServerModelEntryRuntimeModelEntry(t *testing.T) {
	serverModel := config.ServerModelEntry{
		Model:         "provider/model",
		Name:          "Worker model",
		APIURL:        "https://models.example/v1",
		APIKey:        "secret",
		ContextWindow: 32000,
		CallTimeout:   45,
	}
	got := serverModel.RuntimeModelEntry()
	if got.Model != serverModel.Model || got.Name != serverModel.Name ||
		got.APIURL != serverModel.APIURL || got.APIKey != serverModel.APIKey ||
		got.ContextWindow != serverModel.ContextWindow || got.CallTimeout != serverModel.CallTimeout {
		t.Fatalf("RuntimeModelEntry() = %+v, want all server model fields", got)
	}
}
