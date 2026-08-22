package pluginmgr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/core/plugin/archive"
	"github.com/gougoujiang/buildmax/internal/infra/pluginwire"
	"github.com/gougoujiang/buildmax/internal/interface/auth"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

// fakeMarketplace serves one plugin's catalog entry and its packaged bytes.
type fakeMarketplace struct {
	server   *httptest.Server
	releases []model.PluginRelease
	packages map[string][]byte
	// servedDigest overrides the digest header, for a server that disagrees
	// with its own catalog.
	servedDigest string
}

func newFakeMarketplace(t *testing.T) *fakeMarketplace {
	t.Helper()
	m := &fakeMarketplace{packages: map[string][]byte{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/plugins/{plugin_name}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, pluginwire.PluginResponse{
			Plugin:   model.Plugin{PluginID: "pl_test", Name: r.PathValue("plugin_name")},
			Releases: m.releases,
		})
	})
	mux.HandleFunc("GET /api/plugins/{plugin_name}/releases/{version}/download",
		func(w http.ResponseWriter, r *http.Request) {
			data, ok := m.packages[r.PathValue("version")]
			if !ok {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			digest := m.servedDigest
			if digest == "" {
				digest, _ = archive.Digest(bytes.NewReader(data))
			}
			w.Header().Set(pluginwire.DigestHeader, digest)
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(data)
		})
	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// publish adds one release, packaging a minimal plugin under the given name.
func (m *fakeMarketplace) publish(t *testing.T, name, version string, opts ...func(*model.PluginRelease)) model.PluginRelease {
	t.Helper()
	manifest := fmt.Sprintf("name: %s\nversion: %s\n", name, version)
	var buf bytes.Buffer
	if _, err := archive.Pack(&buf, fstest.MapFS{
		"plugin.yaml":            &fstest.MapFile{Data: []byte(manifest)},
		"skills/review/SKILL.md": &fstest.MapFile{Data: []byte("# review\n\nFrom " + version + ".\n")},
	}, archive.Limits{}); err != nil {
		t.Fatal(err)
	}
	digest, err := archive.Digest(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	m.packages[version] = buf.Bytes()

	release := model.PluginRelease{
		PluginID: "pl_test", PluginName: name, Version: version,
		Digest: digest, SizeBytes: int64(buf.Len()), PublishedBy: "u_admin",
		Inspection: model.PluginInspection{Skills: []string{"review"}},
	}
	for _, o := range opts {
		o(&release)
	}
	m.releases = append(m.releases, release)
	return release
}

// signIn writes credentials pointing at the fake server, which is what the
// commands read to decide where a Marketplace is.
func signIn(t *testing.T, serverURL string) {
	t.Helper()
	if err := auth.SaveCredentials(&auth.Credentials{
		ServerURL: serverURL,
		Token:     testsupport.SignJWT("u_reader", "irrelevant-to-the-client"),
		UserID:    "u_reader",
		Email:     "reader@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = auth.Logout() })
}

func marketplaceHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	return home
}

func TestPluginInstallEndToEnd(t *testing.T) {
	home := marketplaceHome(t)
	m := newFakeMarketplace(t)
	signIn(t, m.server.URL)
	release := m.publish(t, "code-review", "1.2.0")

	if err := installForTest(t, Options{Name: "code-review", Version: "", AllowYanked: false, RequireInstalled: false}); err != nil {
		t.Fatalf("install: %v", err)
	}

	// The plugin is where a run would look for it, and loads.
	discovery := config.DiscoverPlugins()
	if len(discovery.Loadable()) != 1 {
		t.Fatalf("discovery = %+v", discovery)
	}
	installed := discovery.Loadable()[0]
	if installed.Name() != "code-review" {
		t.Errorf("installed %q", installed.Name())
	}
	body, err := os.ReadFile(filepath.Join(installed.Path, "skills", "review", "SKILL.md"))
	if err != nil || !strings.Contains(string(body), "From 1.2.0") {
		t.Errorf("payload = %q, %v", body, err)
	}

	// Provenance says which release this copy is, not that it came from a
	// checkout it never had.
	if installed.State.Source != config.PluginSourceMarketplace ||
		installed.State.ReleaseVersion != "1.2.0" || installed.State.Digest != release.Digest {
		t.Errorf("state = %+v", installed.State)
	}
	if installed.State.MarketplaceServer != m.server.URL || installed.State.InstalledAt == 0 {
		t.Errorf("state = %+v", installed.State)
	}
	// Nothing is left staged.
	entries, err := os.ReadDir(filepath.Join(home, "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), stagingPrefix) || strings.HasPrefix(e.Name(), retiredPrefix) {
			t.Errorf("left behind %s", e.Name())
		}
	}
}

// The default takes the newest stable release; naming one takes that one.
func TestPluginInstallSelectsARelease(t *testing.T) {
	marketplaceHome(t)
	m := newFakeMarketplace(t)
	signIn(t, m.server.URL)
	m.publish(t, "code-review", "1.0.0")
	m.publish(t, "code-review", "1.2.0")
	m.publish(t, "code-review", "2.0.0-rc.1")

	if err := installForTest(t, Options{Name: "code-review", Version: "", AllowYanked: false, RequireInstalled: false}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := installedVersion(t); got != "1.2.0" {
		t.Errorf("installed %s, want 1.2.0 rather than the prerelease", got)
	}

	// Asked for the release that is already there, the plan says so rather
	// than downloading it again, and a surface reports that instead of acting.
	session, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := session.Resolve(context.Background(), Options{Name: "code-review"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.AlreadyInstalled || plan.Release.Version != "1.2.0" {
		t.Errorf("plan = %+v", plan)
	}

	// A prerelease is reachable by name.
	if err := installForTest(t, Options{Name: "code-review", Version: "2.0.0-rc.1", AllowYanked: false, RequireInstalled: true}); err != nil {
		t.Fatalf("update to a named prerelease: %v", err)
	}
	if got := installedVersion(t); got != "2.0.0-rc.1" {
		t.Errorf("installed %s", got)
	}
}

// Two checks guard a download, and each catches something the other cannot.
//
// The header says what the server thinks it sent; the catalog record says what
// was published. A server serving different bytes and labelling them honestly
// is caught by comparing those two.
func TestPluginInstallRefusesBytesTheCatalogDoesNotName(t *testing.T) {
	home := marketplaceHome(t)
	m := newFakeMarketplace(t)
	signIn(t, m.server.URL)
	m.publish(t, "code-review", "1.0.0")
	m.packages["1.0.0"] = append(m.packages["1.0.0"], 0x00)

	err := installForTest(t, Options{Name: "code-review", Version: "", AllowYanked: false, RequireInstalled: false})
	if err == nil {
		t.Fatal("a download the catalog does not name should be refused")
	}
	if !strings.Contains(err.Error(), "which the catalog records as") {
		t.Errorf("err = %v, want the header-against-catalog refusal", err)
	}
	assertNothingInstalled(t, home)
}

// A truncation in transit arrives under a header that described the whole
// thing correctly, so only hashing the bytes finds it.
func TestPluginInstallRefusesATruncatedDownload(t *testing.T) {
	home := marketplaceHome(t)
	m := newFakeMarketplace(t)
	signIn(t, m.server.URL)
	release := m.publish(t, "code-review", "1.0.0")
	m.packages["1.0.0"] = m.packages["1.0.0"][:len(m.packages["1.0.0"])-8]
	m.servedDigest = release.Digest

	err := installForTest(t, Options{Name: "code-review", Version: "", AllowYanked: false, RequireInstalled: false})
	if err == nil {
		t.Fatal("a truncated download should be refused")
	}
	if !strings.Contains(err.Error(), "does not match what was published") {
		t.Errorf("err = %v, want the bytes-against-catalog refusal", err)
	}
	assertNothingInstalled(t, home)
}

// A checkout may hold work that exists nowhere else, so installing over one is
// refused rather than resolved.
func TestPluginInstallRefusesToReplaceACheckout(t *testing.T) {
	home := marketplaceHome(t)
	m := newFakeMarketplace(t)
	signIn(t, m.server.URL)
	m.publish(t, "code-review", "1.0.0")
	dir := makeCheckout(t, filepath.Join(home, "plugins", "code-review"), "name: code-review\n")

	err := installForTest(t, Options{Name: "code-review", Version: "", AllowYanked: false, RequireInstalled: false})
	if err == nil || !strings.Contains(err.Error(), "Git checkout") {
		t.Fatalf("err = %v, want a refusal naming the checkout", err)
	}
	// The working tree is untouched.
	if _, statErr := os.Stat(filepath.Join(dir, ".git")); statErr != nil {
		t.Error("the checkout was disturbed")
	}
}

func TestPluginUpdateRequiresAnInstalledPlugin(t *testing.T) {
	marketplaceHome(t)
	m := newFakeMarketplace(t)
	signIn(t, m.server.URL)
	m.publish(t, "code-review", "1.0.0")

	err := installForTest(t, Options{Name: "code-review", Version: "", AllowYanked: false, RequireInstalled: true})
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("err = %v, want a refusal", err)
	}
}

func TestPluginUninstall(t *testing.T) {
	home := marketplaceHome(t)
	m := newFakeMarketplace(t)
	signIn(t, m.server.URL)
	m.publish(t, "code-review", "1.0.0")
	if err := installForTest(t, Options{Name: "code-review", Version: "", AllowYanked: false, RequireInstalled: false}); err != nil {
		t.Fatal(err)
	}

	if err := uninstallForTest(t, "code-review", false); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	assertNothingInstalled(t, home)
	states, err := config.LoadPluginStates(filepath.Join(home, "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	if _, recorded := states.Get("code-review"); recorded {
		t.Error("uninstall left provenance behind")
	}
	if err := uninstallForTest(t, "code-review", false); err == nil {
		t.Error("uninstalling twice should say there is nothing there")
	}
}

// `uninstall` is not the command someone expects to lose uncommitted work to.
func TestPluginUninstallRefusesACheckoutWithoutForce(t *testing.T) {
	home := marketplaceHome(t)
	dir := makeCheckout(t, filepath.Join(home, "plugins", "code-review"), "name: code-review\n")

	err := uninstallForTest(t, "code-review", false)
	if !errors.Is(err, ErrIsCheckout) {
		t.Fatalf("err = %v, want the checkout refusal", err)
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Error("the checkout was removed anyway")
	}
	if err := uninstallForTest(t, "code-review", true); err != nil {
		t.Fatalf("forced uninstall: %v", err)
	}
	if _, statErr := os.Stat(dir); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("a forced uninstall left the checkout")
	}
}

// An interrupted install leaves either the previous plugin or the new one,
// never half of each.
func TestSwapInRestoresThePreviousCopyOnFailure(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, "code-review")
	if err := os.MkdirAll(active, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(active, "plugin.yaml"), []byte("name: code-review\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A staged directory that does not exist cannot be renamed into place.
	if err := swapIn(filepath.Join(root, "absent"), active); err == nil {
		t.Fatal("swapping in a missing directory should fail")
	}
	body, err := os.ReadFile(filepath.Join(active, "plugin.yaml"))
	if err != nil || string(body) != "name: code-review\n" {
		t.Errorf("the previous copy was not restored: %q, %v", body, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("left behind %v", entries)
	}
}

func installedVersion(t *testing.T) string {
	t.Helper()
	loadable := config.DiscoverPlugins().Loadable()
	if len(loadable) != 1 {
		t.Fatalf("got %d loadable plugins, want 1", len(loadable))
	}
	return loadable[0].State.ReleaseVersion
}

func assertNothingInstalled(t *testing.T, home string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(home, "plugins", "code-review")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("something was installed: %v", err)
	}
}

func makeCheckout(t *testing.T, dir, manifest string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "T"},
		{"add", "-A"}, {"commit", "-m", "initial"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// uninstallForTest keeps the tests reading as the command does, now that the
// mechanism moved and only the printing stayed.
func uninstallForTest(t *testing.T, name string, force bool) error {
	t.Helper()
	_, err := Uninstall(name, force)
	return err
}

// installForTest resolves and installs, which is what a surface does either
// side of showing somebody what is about to change.
func installForTest(t *testing.T, opts Options) error {
	t.Helper()
	session, err := Open()
	if err != nil {
		return err
	}
	plan, err := session.Resolve(context.Background(), opts)
	if err != nil {
		return err
	}
	if plan.AlreadyInstalled {
		return errAlreadyInstalled
	}
	return session.Install(context.Background(), opts, plan.Release)
}

// errAlreadyInstalled marks the no-op a surface reports rather than performs.
var errAlreadyInstalled = errors.New("already installed")
