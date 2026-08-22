package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/model"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/core/plugin/archive"
	"github.com/gougoujiang/buildmax/internal/core/plugin/inspect"
	"github.com/gougoujiang/buildmax/internal/infra/git"
	"github.com/gougoujiang/buildmax/internal/interface/auth"
	"github.com/gougoujiang/buildmax/internal/interface/client"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"

	"github.com/spf13/cobra"
)

// Reserved directories inside the plugins directory. Dot-prefixed so discovery
// skips them, which is what lets a half-finished install sit beside working
// plugins without becoming one.
const (
	pluginStagingPrefix = ".staging-"
	pluginRetiredPrefix = ".retired-"
)

func newPluginPublishCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "publish <path>",
		Short: "Pack a plugin directory and publish it to the Marketplace",
		Long: "The version comes from the directory's own plugin.yaml. Publishing an\n" +
			"existing version is refused, so a correction means editing and committing\n" +
			"the manifest rather than replacing what somebody already downloaded.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginPublish(cmd.Context(), cmd.OutOrStdout(), args[0])
		},
	}
}

func newPluginInstallCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "install <name>",
		Short: "Download a plugin from the Marketplace and install it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			version, _ := cmd.Flags().GetString("version")
			allowYanked, _ := cmd.Flags().GetBool("allow-yanked")
			return runPluginInstall(cmd.Context(), cmd.OutOrStdout(), args[0], version, allowYanked, false)
		},
	}
	c.Flags().String("version", "", "install this exact version instead of the newest suitable one")
	c.Flags().Bool("allow-yanked", false, "install a release that was withdrawn")
	return c
}

func newPluginUpdateCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "update <name>",
		Short: "Replace an installed Marketplace plugin with a newer release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			version, _ := cmd.Flags().GetString("version")
			allowYanked, _ := cmd.Flags().GetBool("allow-yanked")
			return runPluginInstall(cmd.Context(), cmd.OutOrStdout(), args[0], version, allowYanked, true)
		},
	}
	c.Flags().String("version", "", "update to this exact version instead of the newest suitable one")
	c.Flags().Bool("allow-yanked", false, "update to a release that was withdrawn")
	return c
}

func newPluginUninstallCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Remove an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			return runPluginUninstall(cmd.OutOrStdout(), args[0], force)
		},
	}
	c.Flags().Bool("force", false, "remove a Git checkout too, discarding anything uncommitted in it")
	return c
}

// marketplaceSession is one authenticated conversation with a server.
type marketplaceSession struct {
	client    *client.Client
	token     string
	serverURL string
}

func openMarketplace() (*marketplaceSession, error) {
	info, err := auth.Info()
	if err != nil {
		return nil, fmt.Errorf("load auth: %w", err)
	}
	if !info.LoggedIn {
		return nil, errors.New("not signed in: run `buildmax login` first")
	}
	token, err := auth.TokenForServer(info.ServerURL)
	if err != nil {
		return nil, err
	}
	return &marketplaceSession{
		client: client.NewClient(info.ServerURL), token: token, serverURL: info.ServerURL,
	}, nil
}

func runPluginPublish(ctx context.Context, w io.Writer, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// Validating here is a courtesy, not the check that matters: the server
	// inspects what it receives. Failing before an upload is just faster.
	pkg, err := inspect.Dir(os.DirFS(abs))
	if err != nil {
		return err
	}
	if errs := coreplugin.Errors(pkg.Findings); len(errs) > 0 {
		for _, f := range errs {
			fmt.Fprintf(w, "  %s\n", f.String())
		}
		return invalidPluginExit()
	}
	if pkg.Manifest.Version == "" {
		return fmt.Errorf("%s has no version, which publishing requires", coreplugin.ManifestFile)
	}

	session, err := openMarketplace()
	if err != nil {
		return err
	}
	body, cleanup, size, err := packForPublish(abs)
	if err != nil {
		return err
	}
	defer cleanup()

	fmt.Fprintf(w, "Publishing %s %s (%s) to %s\n",
		pkg.Manifest.Name, pkg.Manifest.Version, humanBytes(size), session.serverURL)
	release, err := session.client.PublishRelease(ctx, session.token, pkg.Manifest.Name, body, readSourceClaim(ctx, abs))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Published %s %s\n  digest: %s\n", release.PluginName, release.Version, release.Digest)
	if release.Source.Dirty {
		fmt.Fprintln(w, "  packed from a working tree with uncommitted changes, which the record says")
	}
	return nil
}

// packForPublish writes the archive to a temporary file rather than a buffer,
// so publishing a large plugin does not hold it twice in memory.
func packForPublish(dir string) (io.Reader, func(), int64, error) {
	f, err := os.CreateTemp("", "buildmax-publish-*.tar.gz")
	if err != nil {
		return nil, nil, 0, err
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}
	sum, err := archive.Pack(f, os.DirFS(dir), archive.Limits{})
	if err != nil {
		cleanup()
		return nil, nil, 0, fmt.Errorf("pack %s: %w", dir, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, 0, err
	}
	return f, cleanup, sum.Bytes, nil
}

// readSourceClaim describes the checkout the bytes were packed from.
//
// A directory that is not a repository produces an empty claim, which is not an
// error: a package assembled by hand is a legitimate thing to publish, and the
// catalog says so rather than inventing provenance.
func readSourceClaim(ctx context.Context, dir string) model.PluginReleaseSource {
	if !git.IsRepository(dir) {
		return model.PluginReleaseSource{}
	}
	claim := model.PluginReleaseSource{RemoteURL: git.ReadRemoteURL(ctx, dir)}
	if st, err := git.ReadStatus(ctx, dir); err == nil {
		claim.Commit, claim.Branch, claim.Dirty = st.Commit, st.Branch, st.Dirty
	}
	return claim
}

func runPluginInstall(ctx context.Context, w io.Writer, name, version string, allowYanked, requireInstalled bool) error {
	discovery := config.DiscoverPlugins()
	existing, installed := findPlugin(discovery, name)
	if requireInstalled && !installed {
		return fmt.Errorf("%s is not installed; use `buildmax plugin install` instead", name)
	}
	if installed {
		if err := checkReplaceable(existing); err != nil {
			return err
		}
	}

	session, err := openMarketplace()
	if err != nil {
		return err
	}
	detail, err := session.client.GetPlugin(ctx, session.token, name)
	if err != nil {
		return err
	}
	release, err := pluginsvc.SelectRelease(detail.Releases, pluginsvc.SelectOptions{
		Version:       version,
		ClientVersion: config.Version,
		AllowYanked:   allowYanked,
		// Naming a prerelease exactly works; taking one by default does not.
		AllowPrerelease: version != "",
	})
	if err != nil {
		return err
	}
	if installed && existing.State.ReleaseVersion == release.Version && existing.State.Digest == release.Digest {
		fmt.Fprintf(w, "%s %s is already installed.\n", name, release.Version)
		return nil
	}

	fmt.Fprintf(w, "Installing %s %s (%s)\n", name, release.Version, humanBytes(release.SizeBytes))
	writeReleaseSummary(w, *release)

	staged, err := downloadAndStage(ctx, session, name, *release, allowYanked)
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged.root)

	if err := swapIn(staged.extracted, filepath.Join(discovery.Dir, name)); err != nil {
		return err
	}
	if err := recordInstalledRelease(discovery.Dir, name, session.serverURL, *release, installed); err != nil {
		return err
	}
	fmt.Fprintf(w, "Installed %s %s. A run already in flight keeps the plugins it started with.\n",
		name, release.Version)
	return nil
}

// checkReplaceable refuses to overwrite something BuildMax did not put there.
//
// A Git checkout is somebody's working tree: replacing it would discard work
// that only exists locally, and the design refuses rather than guess. The user
// removes or renames it themselves.
func checkReplaceable(existing config.DiscoveredPlugin) error {
	if git.IsRepository(existing.Path) {
		return fmt.Errorf("%s is a Git checkout at %s.\n"+
			"Installing would replace it. Remove or rename it first if that is what you want",
			existing.Name(), existing.Path)
	}
	if existing.StateKnown && existing.State.Source == config.PluginSourceMarketplace {
		return nil
	}
	if !existing.StateKnown || existing.State.Source == config.PluginSourceLocal {
		return fmt.Errorf("%s at %s was not installed from the Marketplace.\n"+
			"Remove it first if you want the published release instead",
			existing.Name(), existing.Path)
	}
	return nil
}

// stagedInstall is one download on its way in.
type stagedInstall struct {
	root      string
	extracted string
}

func downloadAndStage(ctx context.Context, session *marketplaceSession, name string, release model.PluginRelease, allowYanked bool) (stagedInstall, error) {
	pluginsDir := config.PluginsDir()
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return stagedInstall{}, err
	}
	root, err := os.MkdirTemp(pluginsDir, pluginStagingPrefix)
	if err != nil {
		return stagedInstall{}, err
	}
	staged := stagedInstall{root: root, extracted: filepath.Join(root, "extract")}

	archivePath := filepath.Join(root, "package.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		os.RemoveAll(root)
		return stagedInstall{}, err
	}
	served, err := session.client.DownloadRelease(ctx, session.token, name, release.Version, allowYanked, f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.RemoveAll(root)
		return stagedInstall{}, err
	}
	// Two checks, not one. The header says what the server thinks it sent; the
	// catalog record says what was published. A download is only right when it
	// matches both, and comparing the bytes to the record is what catches a
	// truncation the header would have described correctly.
	if served != "" && served != release.Digest {
		os.RemoveAll(root)
		return stagedInstall{}, fmt.Errorf("the server sent digest %s for %s %s, which the catalog records as %s",
			served, name, release.Version, release.Digest)
	}
	if err := verifyStagedDigest(archivePath, release.Digest); err != nil {
		os.RemoveAll(root)
		return stagedInstall{}, err
	}

	if err := extractAndValidate(archivePath, staged.extracted, name); err != nil {
		os.RemoveAll(root)
		return stagedInstall{}, err
	}
	return staged, nil
}

func verifyStagedDigest(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := archive.VerifyDigest(f, want); err != nil {
		return fmt.Errorf("the downloaded package does not match what was published: %w", err)
	}
	return nil
}

// extractAndValidate unpacks a verified package and reads it the same way a run
// would, so a package that would not load never reaches the plugins directory.
func extractAndValidate(archivePath, dest, name string) error {
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
		return fmt.Errorf("the downloaded package would not load: %s", errs[0].String())
	}
	if pkg.Manifest.Name != name {
		return fmt.Errorf("the downloaded package names %q, not %q", pkg.Manifest.Name, name)
	}
	return nil
}

// swapIn exchanges the staged directory with the active one.
//
// The old directory is moved aside before the new one lands and removed only
// once it has, so an interrupted install leaves either the previous plugin or
// the new one — never half of each.
func swapIn(staged, active string) error {
	retired := ""
	if _, err := os.Stat(active); err == nil {
		retired = filepath.Join(filepath.Dir(active), pluginRetiredPrefix+filepath.Base(active))
		_ = os.RemoveAll(retired)
		if err := os.Rename(active, retired); err != nil {
			return fmt.Errorf("set aside the installed copy: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.Rename(staged, active); err != nil {
		if retired != "" {
			// Put back what was working before reporting the failure.
			_ = os.Rename(retired, active)
		}
		return fmt.Errorf("install: %w", err)
	}
	if retired != "" {
		// Nothing is kept for rollback: an exact release can always be
		// downloaded again, and a second copy on disk raises a question about
		// which one is current that nothing here would answer.
		_ = os.RemoveAll(retired)
	}
	return nil
}

func recordInstalledRelease(pluginsDir, name, serverURL string, release model.PluginRelease, wasInstalled bool) error {
	now := time.Now().Unix()
	return config.UpdatePluginStates(pluginsDir, func(s *config.PluginStates) error {
		st, _ := s.Get(name)
		st.Source = config.PluginSourceMarketplace
		st.MarketplaceServer = serverURL
		st.CatalogID = release.PluginID
		st.ReleaseVersion = release.Version
		st.Digest = release.Digest
		// Repository fields would describe a checkout this copy is not.
		st.RepositoryURL, st.LastCommit = "", ""
		if !wasInstalled || st.InstalledAt == 0 {
			st.InstalledAt = now
		}
		st.UpdatedAt = now
		s.Set(name, st)
		return nil
	})
}

func runPluginUninstall(w io.Writer, name string, force bool) error {
	discovery := config.DiscoverPlugins()
	found, ok := findPlugin(discovery, name)
	if !ok {
		return fmt.Errorf("no plugin named %q under %s", name, discovery.Dir)
	}
	// A checkout may hold work that exists nowhere else, and `uninstall` is not
	// the command someone expects to lose it to.
	if git.IsRepository(found.Path) && !force {
		return fmt.Errorf("%s is a Git checkout at %s.\n"+
			"Remove it yourself, or pass --force to delete it and anything uncommitted in it",
			found.Name(), found.Path)
	}
	if err := os.RemoveAll(found.Path); err != nil {
		return err
	}
	if err := config.UpdatePluginStates(discovery.Dir, func(s *config.PluginStates) error {
		s.Remove(found.Dir)
		return nil
	}); err != nil {
		return err
	}
	fmt.Fprintf(w, "Removed %s from %s.\n", found.Name(), discovery.Dir)
	return nil
}

// writeReleaseSummary shows what a release contributes before it is installed,
// which is the moment the decision is still open.
func writeReleaseSummary(w io.Writer, release model.PluginRelease) {
	if release.Digest != "" {
		fmt.Fprintf(w, "  digest:      %s\n", release.Digest)
	}
	if release.PublishedBy != "" {
		fmt.Fprintf(w, "  published by %s\n", release.PublishedBy)
	}
	insp := release.Inspection
	if len(insp.Skills) > 0 {
		fmt.Fprintf(w, "  skills:      %v\n", insp.Skills)
	}
	if len(insp.Subagents) > 0 {
		names := make([]string, 0, len(insp.Subagents))
		for _, s := range insp.Subagents {
			names = append(names, s.Name)
		}
		fmt.Fprintf(w, "  subagents:   %v\n", names)
	}
	for _, s := range insp.MCP {
		fmt.Fprintf(w, "  mcp server:  %s (%s %s%s)\n", s.ID, s.Transport, s.Executable, s.Host)
	}
	for _, h := range insp.Hooks {
		fmt.Fprintf(w, "  hook:        %s %s %s%s\n", h.Event, h.Type, h.Executable, h.Host)
	}
	if len(insp.EnvRefs) > 0 {
		var missing []string
		for _, name := range insp.EnvRefs {
			if _, set := os.LookupEnv(name); !set {
				missing = append(missing, name)
			}
		}
		fmt.Fprintf(w, "  environment: %v\n", insp.EnvRefs)
		if len(missing) > 0 {
			fmt.Fprintf(w, "  NOT SET:     %v\n", missing)
		}
	}
	if release.Source.Dirty {
		fmt.Fprintln(w, "  packed from a working tree with uncommitted changes")
	}
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}
