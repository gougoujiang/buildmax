package clie2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/server/handlers"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

// The Marketplace loop is only real when it crosses every boundary at once: the
// released binary packs a directory, a server it does not share memory with
// hashes and inspects the bytes, and a second machine's BUILDMAX_HOME installs
// the result by name. Everything below the HTTP boundary is the production
// code path; only the stores are in memory.
//
// See docs/design/plugin-marketplace.md §12, Phase B acceptance.

const marketplaceSecret = "e2e-marketplace-secret"

const (
	publisherID = "u_publisher"
	consumerID  = "u_consumer"
)

// startMarketplace serves the real catalog routes over in-memory stores and a
// package store on disk.
func startMarketplace(t *testing.T) *httptest.Server {
	t.Helper()
	users := &mock.MockUserStore{ByID: map[string]*model.User{}, ByEmail: map[string]*model.User{}}
	for id, email := range map[string]string{
		publisherID: "publisher@example.com",
		consumerID:  "consumer@example.com",
	} {
		u := &model.User{ID: id, Email: email, CreatedAt: 1}
		users.ByID[id] = u
		users.ByEmail[email] = u
	}
	// Only the publisher holds the grant. The consumer installing without one
	// is the authority split the design draws, tested rather than assumed.
	grants := &mock.MockSystemGrantStore{}
	grants.GrantForTest(publisherID, model.SystemRoleAdmin)
	audits := &mock.MockAuditStore{}

	h := handlers.NewHandler(handlers.Config{
		JWTSecret:        marketplaceSecret,
		UserStore:        users,
		TeamStore:        &mock.MockTeamStore{},
		SystemGrantStore: grants,
		AuditStore:       audits,
		Audit:            audit.NewRecorder(audits),
		PluginService: &pluginsvc.Service{
			Catalog:   mock.NewMockPluginStore(),
			Packages:  objectstore.NewLocalFSPluginPackageStorage(t.TempDir()),
			KeyPrefix: "e2e",
			Audit:     audit.NewRecorder(audits),
		},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// signedInHome returns a BUILDMAX_HOME the binary can publish or install from.
func signedInHome(t *testing.T, serverURL, userID string) string {
	t.Helper()
	home := t.TempDir()
	creds := map[string]any{
		"server_url": serverURL,
		"token":      testsupport.SignJWT(userID, marketplaceSecret),
		"user_id":    userID,
		"email":      userID + "@example.com",
		"saved_at":   time.Now().Unix(),
	}
	data, err := json.Marshal(creds)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "auth.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

// writePluginSource lays out a plugin an author would publish.
func writePluginSource(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"plugin.yaml": "name: e2e-review\nversion: " + version + "\n" +
			"description: End-to-end review helpers.\ndisplay_name: E2E Review\n",
		"skills/review/SKILL.md": "# review\n\nReview a change, version " + version + ".\n",
		"agents/reviewer.md": "---\nname: e2e-reviewer\ndescription: Reviews changes.\n" +
			"tools: Read, Grep\n---\n\nYou review.\n",
	}
	for rel, body := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

var digestPattern = regexp.MustCompile(`sha256:[0-9a-f]{64}`)

func TestMarketplacePublishThenInstall(t *testing.T) {
	server := startMarketplace(t)
	source := writePluginSource(t, "1.0.0")
	publisherHome := signedInHome(t, server.URL, publisherID)
	consumerHome := signedInHome(t, server.URL, consumerID)

	published := runPlugin(t, publisherHome, "publish", source)
	if published.exitCode != 0 {
		t.Fatalf("publish exited %d\n%s%s", published.exitCode, published.stdout, published.stderr)
	}
	publishedDigest := digestPattern.FindString(published.stdout)
	if publishedDigest == "" {
		t.Fatalf("publish printed no digest:\n%s", published.stdout)
	}

	// A second account, holding no grant, installs by name.
	installed := runPlugin(t, consumerHome, "install", "e2e-review")
	if installed.exitCode != 0 {
		t.Fatalf("install exited %d\n%s%s", installed.exitCode, installed.stdout, installed.stderr)
	}
	if !strings.Contains(installed.stdout, "Installed e2e-review 1.0.0") {
		t.Errorf("install output:\n%s", installed.stdout)
	}

	// The payload is on disk where a run would look for it.
	skill := filepath.Join(consumerHome, "plugins", "e2e-review", "skills", "review", "SKILL.md")
	body, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}
	if !strings.Contains(string(body), "version 1.0.0") {
		t.Errorf("installed skill = %q", body)
	}

	// Both sides report the same digest, which is Phase B's acceptance.
	status := runPlugin(t, consumerHome, "status", "e2e-review")
	if status.exitCode != 0 {
		t.Fatalf("status exited %d\n%s%s", status.exitCode, status.stdout, status.stderr)
	}
	if got := digestPattern.FindString(status.stdout); got != publishedDigest {
		t.Errorf("installed digest %q, published %q\n%s", got, publishedDigest, status.stdout)
	}
	for _, want := range []string{"marketplace 1.0.0", "e2e-reviewer", "review"} {
		if !strings.Contains(status.stdout, want) {
			t.Errorf("status is missing %q:\n%s", want, status.stdout)
		}
	}
}

// Publishing changes what every member of the deployment can install, so an
// account without the grant is refused before any bytes are stored.
func TestMarketplacePublishNeedsAGrant(t *testing.T) {
	server := startMarketplace(t)
	source := writePluginSource(t, "1.0.0")
	consumerHome := signedInHome(t, server.URL, consumerID)

	published := runPlugin(t, consumerHome, "publish", source)
	if published.exitCode == 0 {
		t.Fatalf("publishing without a grant should fail\n%s", published.stdout)
	}
	// And nothing reached the catalog for anyone else to install.
	installed := runPlugin(t, consumerHome, "install", "e2e-review")
	if installed.exitCode == 0 {
		t.Errorf("a refused publish still produced something installable\n%s", installed.stdout)
	}
}

// A correction is a new version, not a replacement of what somebody already
// downloaded.
func TestMarketplaceRepublishIsRefusedAndUpdateTakesTheNewVersion(t *testing.T) {
	server := startMarketplace(t)
	publisherHome := signedInHome(t, server.URL, publisherID)
	consumerHome := signedInHome(t, server.URL, consumerID)

	first := writePluginSource(t, "1.0.0")
	if r := runPlugin(t, publisherHome, "publish", first); r.exitCode != 0 {
		t.Fatalf("publish 1.0.0: %s%s", r.stdout, r.stderr)
	}
	if r := runPlugin(t, publisherHome, "publish", first); r.exitCode == 0 {
		t.Errorf("republishing one version should be refused\n%s", r.stdout)
	}
	if r := runPlugin(t, consumerHome, "install", "e2e-review"); r.exitCode != 0 {
		t.Fatalf("install: %s%s", r.stdout, r.stderr)
	}

	second := writePluginSource(t, "1.1.0")
	if r := runPlugin(t, publisherHome, "publish", second); r.exitCode != 0 {
		t.Fatalf("publish 1.1.0: %s%s", r.stdout, r.stderr)
	}
	update := runPlugin(t, consumerHome, "update", "e2e-review")
	if update.exitCode != 0 {
		t.Fatalf("update exited %d\n%s%s", update.exitCode, update.stdout, update.stderr)
	}
	skill := filepath.Join(consumerHome, "plugins", "e2e-review", "skills", "review", "SKILL.md")
	body, err := os.ReadFile(skill)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "version 1.1.0") {
		t.Errorf("the update did not replace the payload: %q", body)
	}
}

// runPlugin drives one `buildmax plugin` invocation against a home.
//
// It does not go through run(): these commands take no --workspace, and one
// that did would be answering a different question.
func runPlugin(t *testing.T, home string, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(binary, append([]string{"plugin"}, args...)...)
	cmd.Env = append(os.Environ(), "BUILDMAX_HOME="+home)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := runResult{stdout: stdout.String(), stderr: stderr.String()}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case asExitError(err, &exitErr):
		result.exitCode = exitErr.ExitCode()
	default:
		t.Fatalf("run %s plugin %v: %v", binary, args, err)
	}
	return result
}
