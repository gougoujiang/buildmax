package architecture_test

// Deployment-manifest constraints. The shipped Kubernetes manifest configures the
// server through a server.yaml carried in a ConfigMap. Nothing else checks that
// this file still matches what the code reads, and the last time the two drifted
// the deployment crash-looped for every user of `./make kind up` without any test
// noticing. These tests close that gap.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/gougoujiang/buildmax/internal/config"
)

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above working directory")
		}
		dir = parent
	}
}

// manifestServerYAML extracts data["server.yaml"] from the buildmax-config
// ConfigMap in the deployment manifest.
func manifestServerYAML(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "deployment", "buildmax-deploy.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read deployment manifest: %v", err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(body)))
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Data map[string]string `yaml:"data"`
		}
		if err := dec.Decode(&doc); err != nil {
			break
		}
		if doc.Kind == "ConfigMap" && doc.Metadata.Name == "buildmax-config" {
			body, ok := doc.Data["server.yaml"]
			if !ok {
				t.Fatal("buildmax-config ConfigMap has no server.yaml key")
			}
			return body
		}
	}
	t.Fatal("no buildmax-config ConfigMap found in deployment/buildmax-deploy.yaml")
	return ""
}

// TestDeploymentConfigMapLoads fails when the server.yaml shipped in the
// deployment ConfigMap no longer parses into the config the server reads, or
// when it stops pointing at in-cluster services. A manifest that falls back to
// defaults sends the server to MySQL on localhost, which is exactly the failure
// this guards against.
func TestDeploymentConfigMapLoads(t *testing.T) {
	root := repoRoot(t)
	body := manifestServerYAML(t, root)

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "server.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write server.yaml: %v", err)
	}
	t.Setenv(config.EnvKeyBuildmaxHome, home)

	cfg, err := config.LoadServerConfig()
	if err != nil {
		t.Fatalf("the deployment ConfigMap does not load as a server config: %v", err)
	}

	if cfg.Database.Host == "" || cfg.Database.Host == "localhost" {
		t.Errorf("database.host = %q; the manifest must point at the in-cluster MySQL", cfg.Database.Host)
	}
	if cfg.Storage.PersistBackend == config.ProviderLocalFS || cfg.Storage.ArtifactBackend == config.ProviderLocalFS {
		t.Errorf("storage backends = %q/%q; the manifest must use shared storage, not the pod filesystem",
			cfg.Storage.PersistBackend, cfg.Storage.ArtifactBackend)
	}
	if cfg.Storage.MinIO.Endpoint == "" || strings.Contains(cfg.Storage.MinIO.Endpoint, "localhost") {
		t.Errorf("storage.minio.endpoint = %q; must be reachable from server and worker pods", cfg.Storage.MinIO.Endpoint)
	}
	if cfg.Worker.ServerURL == "" || strings.Contains(cfg.Worker.ServerURL, "localhost") {
		t.Errorf("worker.server_url = %q; worker pods cannot reach the server on localhost", cfg.Worker.ServerURL)
	}
	if cfg.Worker.RunMode == "k8s_job" && cfg.Worker.K8s.ConfigMap == "" {
		t.Error("worker.run_mode is k8s_job but worker.k8s.config_map is empty; worker pods would get no server.yaml")
	}
}

// TestDeploymentConfigMapCarriesNoSecrets keeps credentials out of the ConfigMap.
// A ConfigMap is not a Secret; these fields are injected from the environment.
func TestDeploymentConfigMapCarriesNoSecrets(t *testing.T) {
	root := repoRoot(t)
	body := manifestServerYAML(t, root)

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "server.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write server.yaml: %v", err)
	}
	t.Setenv(config.EnvKeyBuildmaxHome, home)

	cfg, err := config.LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig: %v", err)
	}
	secrets := map[string]string{
		"jwt_secret":                 cfg.JWTSecret,
		"dev_login_otp":              cfg.DevLoginOTP,
		"worker.token":               cfg.Worker.Token,
		"conversation.model.api_key": cfg.Conversation.Model.APIKey,
	}
	for field, value := range secrets {
		if value != "" {
			t.Errorf("%s is set in the deployment ConfigMap; credentials belong in buildmax-secret", field)
		}
	}
	// The MinIO and database defaults are non-empty by design, so assert the
	// manifest does not restate them rather than that they are unset.
	if strings.Contains(body, "secret_key:") || strings.Contains(body, "password:") {
		t.Error("the deployment ConfigMap declares a password or secret_key; inject those from buildmax-secret")
	}
}

// TestDeploymentSecretKeysMatchEnvSpec fails when the example Secret offers a key
// the code does not read, or omits one the manifest wires up.
func TestDeploymentSecretKeysMatchEnvSpec(t *testing.T) {
	root := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "deployment", "buildmax-secret.example.yaml"))
	if err != nil {
		t.Fatalf("read secret example: %v", err)
	}
	var doc struct {
		StringData map[string]string `yaml:"stringData"`
	}
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("parse secret example: %v", err)
	}

	known := make(map[string]bool, len(config.EnvVars))
	for _, ev := range config.EnvVars {
		known[ev.Name] = true
	}
	for key := range doc.StringData {
		if !known[key] {
			t.Errorf("buildmax-secret.example.yaml offers %s, which no BuildMax binary reads", key)
		}
	}
}

func TestDeploymentSmokeConfigsLoadWithoutSecrets(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		name    string
		file    string
		runMode string
	}{
		{name: "compose", file: "server.compose.yaml", runMode: "local_process"},
		{name: "kind", file: "server.kind.yaml", runMode: "k8s_job"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(root, "deployment", "smoke", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			if err := os.WriteFile(filepath.Join(home, "server.yaml"), body, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv(config.EnvKeyBuildmaxHome, home)
			t.Setenv(config.EnvKeyBuildmaxConversationAPIKey, "")
			cfg, err := config.LoadServerConfig()
			if err != nil {
				t.Fatalf("LoadServerConfig: %v", err)
			}
			if cfg.Worker.RunMode != tt.runMode {
				t.Errorf("worker.run_mode = %q, want %q", cfg.Worker.RunMode, tt.runMode)
			}
			if !strings.Contains(cfg.Conversation.Model.APIURL, "smoke") && !strings.Contains(cfg.Conversation.Model.APIURL, "mock-llm") {
				t.Errorf("conversation model URL %q does not target the smoke service", cfg.Conversation.Model.APIURL)
			}
			if cfg.Conversation.Model.APIKey != "" || cfg.Worker.Token != "" || cfg.JWTSecret != "" {
				t.Error("smoke server config contains credentials; inject them at runtime")
			}
		})
	}
}
