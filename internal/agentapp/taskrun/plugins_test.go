package taskrun

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	archive "github.com/gougoujiang/buildmax/internal/infra/pluginarchive"
)

func mapFile(body string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(body)} }

// packBytes builds a real package so the guards under test see real bytes
// rather than a stub that agrees with them.
func packBytes(t *testing.T, files fstest.MapFS) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	sum, err := archive.Pack(&buf, files, archive.Limits{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	return buf.Bytes(), sum.Digest
}

func skillPackage(t *testing.T, name string) ([]byte, string) {
	t.Helper()
	return packBytes(t, fstest.MapFS{
		"plugin.yaml":            mapFile("name: " + name + "\nversion: 1.0.0\ndescription: A plugin.\n"),
		"skills/review/SKILL.md": mapFile("---\nname: review\ndescription: Reviews things.\n---\n\n# review\n"),
	})
}

// serveBytes is a fetcher that hands back what the caller staged, with the
// digest the server would have sent.
func serveBytes(data []byte, digestHeader string) packageFetcher {
	return func(_ context.Context, _, _ string, w *os.File) (string, error) {
		if _, err := w.Write(data); err != nil {
			return "", err
		}
		return digestHeader, nil
	}
}

func TestMaterializePlacesAVerifiedPackage(t *testing.T) {
	globalDir := t.TempDir()
	data, digest := skillPackage(t, "code-review")
	pins := []coreplugin.Pin{{PluginName: "code-review", Version: "1.0.0", Digest: digest}}

	if err := materializePlugins(context.Background(), globalDir, pins, serveBytes(data, digest)); err != nil {
		t.Fatalf("materializePlugins: %v", err)
	}
	manifest := filepath.Join(globalDir, "plugins", "code-review", "plugin.yaml")
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("the plugin is not where a run would discover it: %v", err)
	}
	skill := filepath.Join(globalDir, "plugins", "code-review", "skills", "review", "SKILL.md")
	if _, err := os.Stat(skill); err != nil {
		t.Errorf("the package contents did not land: %v", err)
	}
}

// The staged archive must not survive: a leftover download in the plugins
// directory is a file the next step would have to explain.
func TestMaterializeLeavesNoStagedArchive(t *testing.T) {
	globalDir := t.TempDir()
	data, digest := skillPackage(t, "code-review")
	pins := []coreplugin.Pin{{PluginName: "code-review", Version: "1.0.0", Digest: digest}}

	if err := materializePlugins(context.Background(), globalDir, pins, serveBytes(data, digest)); err != nil {
		t.Fatalf("materializePlugins: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(globalDir, "plugins"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".download-") {
			t.Errorf("a staged archive was left behind: %s", e.Name())
		}
	}
}

// Bytes that do not hash to the pinned digest fail the run.
func TestMaterializeRefusesAMismatchedDigest(t *testing.T) {
	globalDir := t.TempDir()
	data, digest := skillPackage(t, "code-review")
	pins := []coreplugin.Pin{{PluginName: "code-review", Version: "1.0.0", Digest: digest}}

	tampered := append([]byte{}, data...)
	tampered[len(tampered)-1] ^= 0xff
	err := materializePlugins(context.Background(), globalDir, pins, serveBytes(tampered, digest))
	if err == nil {
		t.Fatal("tampered bytes were accepted")
	}
	if _, statErr := os.Stat(filepath.Join(globalDir, "plugins", "code-review")); statErr == nil {
		t.Error("a refused package still became a plugin directory")
	}
}

// The pin decides, not the header the server sent with the bytes.
func TestMaterializeRefusesADigestTheServerDisagreesWith(t *testing.T) {
	globalDir := t.TempDir()
	data, digest := skillPackage(t, "code-review")
	pins := []coreplugin.Pin{{PluginName: "code-review", Version: "1.0.0", Digest: digest}}

	err := materializePlugins(context.Background(), globalDir, pins, serveBytes(data, "sha256:something-else"))
	if err == nil {
		t.Fatal("a served digest disagreeing with the pin was accepted")
	}
	if !strings.Contains(err.Error(), "pinned") {
		t.Errorf("err = %v, want it to say the pin disagreed", err)
	}
}

// A package that names a different plugin is not the one that was activated.
func TestMaterializeRefusesAMisnamedPackage(t *testing.T) {
	globalDir := t.TempDir()
	data, digest := skillPackage(t, "something-else")
	pins := []coreplugin.Pin{{PluginName: "code-review", Version: "1.0.0", Digest: digest}}

	err := materializePlugins(context.Background(), globalDir, pins, serveBytes(data, digest))
	if err == nil {
		t.Fatal("a package naming another plugin was accepted")
	}
	if _, statErr := os.Stat(filepath.Join(globalDir, "plugins", "code-review")); statErr == nil {
		t.Error("a refused package still became a plugin directory")
	}
}

// A package the runtime would reject never reaches the plugins directory.
func TestMaterializeRefusesAPackageThatWouldNotLoad(t *testing.T) {
	globalDir := t.TempDir()
	data, digest := packBytes(t, fstest.MapFS{
		"plugin.yaml": mapFile("name: code-review\nversion: not-a-version\n"),
	})
	pins := []coreplugin.Pin{{PluginName: "code-review", Version: "1.0.0", Digest: digest}}

	if err := materializePlugins(context.Background(), globalDir, pins, serveBytes(data, digest)); err == nil {
		t.Fatal("a package that would not load was accepted")
	}
}

// One bad pin fails the whole run rather than leaving it with the others.
func TestOneFailedPinFailsTheRun(t *testing.T) {
	globalDir := t.TempDir()
	good, goodDigest := skillPackage(t, "code-review")
	pins := []coreplugin.Pin{
		{PluginName: "code-review", Version: "1.0.0", Digest: goodDigest},
		{PluginName: "audit", Version: "1.0.0", Digest: "sha256:never-matches"},
	}
	fetch := func(_ context.Context, name, _ string, w *os.File) (string, error) {
		if name == "code-review" {
			_, err := w.Write(good)
			return goodDigest, err
		}
		_, err := w.Write([]byte("not a package"))
		return "", err
	}

	err := materializePlugins(context.Background(), globalDir, pins, fetch)
	if err == nil {
		t.Fatal("a run continued with one plugin missing")
	}
	if !strings.Contains(err.Error(), "audit") {
		t.Errorf("err = %v, want it to name the plugin that failed", err)
	}
}

// No pins is the common case: an agent that names nothing, or no agent at all.
func TestMaterializeWithNoPinsDoesNothing(t *testing.T) {
	globalDir := t.TempDir()
	if err := materializePlugins(context.Background(), globalDir, nil, nil); err != nil {
		t.Fatalf("materializePlugins: %v", err)
	}
	if _, err := os.Stat(filepath.Join(globalDir, "plugins")); err == nil {
		t.Error("a run with no plugins created a plugins directory")
	}
}
