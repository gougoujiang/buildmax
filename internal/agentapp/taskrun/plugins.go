package taskrun

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gougoujiang/buildmax/internal/core/model"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/core/plugin/archive"
	"github.com/gougoujiang/buildmax/internal/core/plugin/inspect"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
)

// A worker's BUILDMAX_HOME is created fresh per run, so its plugins directory
// starts empty and nothing loads. Materializing fills it before the runtime is
// assembled, which is why agentapp needs no change: it discovers plugins from
// BUILDMAX_HOME as it always has.
//
// Everything below the download reuses what the local install already does —
// internal/core/plugin/archive for extraction with its traversal, link,
// duplicate-path, and size guards, and .../inspect for the check that decides
// whether a package would load. A second implementation for workers would be a
// second set of rules about what an archive may contain.

// packageFetcher writes one release's bytes and returns the digest the server
// sent with them. It is a function so a test can materialize without a server.
type packageFetcher func(ctx context.Context, name, version string, w *os.File) (string, error)

// httpPackageFetcher downloads over the run-scoped worker route.
func httpPackageFetcher(cfg workerclient.WorkerAPIClientConfig, taskRunID string) packageFetcher {
	return func(ctx context.Context, name, version string, w *os.File) (string, error) {
		return workerclient.DownloadPluginPackage(ctx, cfg, taskRunID, name, version, w)
	}
}

// materializePlugins places every pinned release under globalDir/plugins.
//
// A pin that fails to download, fails its digest, or would not load fails the
// whole run rather than starting it without that plugin. A background run's
// output is acted on by somebody who was not watching it, so silently doing
// less than the team activated is the wrong failure.
func materializePlugins(ctx context.Context, globalDir string, pins []model.PluginPin, fetch packageFetcher) error {
	if len(pins) == 0 {
		return nil
	}
	pluginsDir := filepath.Join(globalDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return fmt.Errorf("create the run's plugins directory: %w", err)
	}
	for _, pin := range pins {
		if err := materializeOne(ctx, pluginsDir, pin, fetch); err != nil {
			return fmt.Errorf("plugin %s@%s: %w", pin.PluginName, pin.Version, err)
		}
	}
	return nil
}

func materializeOne(ctx context.Context, pluginsDir string, pin model.PluginPin, fetch packageFetcher) error {
	// The archive lands in the run's own directory rather than the system
	// temp: a run is already given a place to write, and keeping the bytes
	// there means a killed worker leaves nothing behind anywhere else.
	staged, err := os.CreateTemp(pluginsDir, ".download-*")
	if err != nil {
		return fmt.Errorf("stage the package: %w", err)
	}
	stagedPath := staged.Name()
	defer func() {
		staged.Close()
		_ = os.Remove(stagedPath)
	}()

	served, err := fetch(ctx, pin.PluginName, pin.Version, staged)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("finish writing the package: %w", err)
	}
	// The pin's digest decides, not the header. The header is a convenience the
	// server offers; the pin is what the team activated, and a server that sent
	// a different one has already disagreed with the record.
	if served != "" && served != pin.Digest {
		return fmt.Errorf("the server served digest %s where this run is pinned to %s", served, pin.Digest)
	}
	if err := verifyPackageDigest(stagedPath, pin.Digest); err != nil {
		return err
	}

	dest := filepath.Join(pluginsDir, pin.PluginName)
	if err := extractPackage(stagedPath, dest, pin.PluginName); err != nil {
		// A half-extracted directory would be discovered on the next step as a
		// broken plugin, so it goes rather than staying to confuse the failure.
		_ = os.RemoveAll(dest)
		return err
	}
	return nil
}

func verifyPackageDigest(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := archive.VerifyDigest(f, want); err != nil {
		return fmt.Errorf("the package does not match the pinned digest: %w", err)
	}
	return nil
}

// extractPackage unpacks a verified package and reads it the way a run would,
// so a package that would not load never becomes a plugin directory.
func extractPackage(archivePath, dest, name string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := archive.Extract(f, dest, archive.Limits{}); err != nil {
		return fmt.Errorf("unpack: %w", err)
	}
	pkg, err := inspect.Dir(os.DirFS(dest))
	if err != nil {
		return err
	}
	if errs := coreplugin.Errors(pkg.Findings); len(errs) > 0 {
		return fmt.Errorf("the package would not load: %s", errs[0].String())
	}
	if pkg.Manifest.Name != name {
		return fmt.Errorf("the package names %q, not %q", pkg.Manifest.Name, name)
	}
	return nil
}
