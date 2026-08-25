// Package plugin owns publication and the catalog lifecycle for the private
// Marketplace.
//
// The server's half of publishing is deliberately independent of the client's:
// it hashes, extracts, and inspects the bytes it received rather than trusting
// what the publisher said about them. A client that validated is a convenience;
// this is the check that decides what a deployment's members can install.
package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	archive "github.com/gougoujiang/buildmax/internal/infra/pluginarchive"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	inspect "github.com/gougoujiang/buildmax/internal/service/plugininspect"
)

// ErrInvalidPackage means the uploaded bytes are not a plugin this deployment
// would be able to load. It is a refusal of the request, not a server fault.
var ErrInvalidPackage = errors.New("invalid plugin package")

// ErrNameMismatch means the manifest inside the package names a different
// plugin than the route it was published to.
var ErrNameMismatch = errors.New("the package names a different plugin")

// digestPrefixLen is how much of a digest reaches the audit trail: enough to
// tell two releases apart when reading the record, and short enough that the
// trail is not a second copy of the catalog.
const digestPrefixLen = 12

// Service publishes releases and manages catalog entries.
type Service struct {
	Catalog CatalogStore
	// Activations and Teams carry the team half of distribution: which
	// releases a team's background runs may use, and who fills that list.
	// They are nil in a deployment that only publishes and installs locally.
	Activations ActivationStore
	Teams       model.TeamStore
	Packages    PackageStore
	// KeyPrefix scopes package keys inside the object store.
	KeyPrefix string
	Audit     *audit.Recorder
	// Limits bound what one upload may cost. The zero value takes the defaults.
	Limits archive.Limits
}

// CreateEntryInput reserves a catalog name.
type CreateEntryInput struct {
	Name        string
	DisplayName string
	Description string
	ActorID     string
}

// CreateEntry adds a catalog entry.
func (s *Service) CreateEntry(ctx context.Context, in CreateEntryInput) (*model.Plugin, error) {
	if err := coreplugin.ValidateName(in.Name); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPackage, err)
	}
	entry, err := s.Catalog.CreatePlugin(ctx, model.CreatePluginInput{
		Name:        in.Name,
		DisplayName: in.DisplayName,
		Description: in.Description,
		CreatedBy:   in.ActorID,
	})
	if err != nil {
		return nil, err
	}
	s.record(ctx, in.ActorID, model.AuditPluginCreated, entry.Name, entry.Name, "")
	return entry, nil
}

// UpdateEntry changes display metadata. The name is not editable: it identifies
// the plugin every installed copy came from.
func (s *Service) UpdateEntry(ctx context.Context, name string, in model.UpdatePluginInput, actorID string) (*model.Plugin, error) {
	entry, err := s.Catalog.UpdatePlugin(ctx, name, in)
	if err != nil {
		return nil, err
	}
	s.record(ctx, actorID, model.AuditPluginUpdated, entry.Name, entry.Name, "")
	return entry, nil
}

// SetArchived retires or restores a catalog entry.
//
// It hides the entry and refuses new releases. It deletes nothing: a copy
// somebody already installed keeps working, and the record still explains where
// that copy came from.
func (s *Service) SetArchived(ctx context.Context, name string, archived bool, actorID string) error {
	entry, err := s.Catalog.GetPlugin(ctx, name)
	if err != nil {
		return err
	}
	if entry == nil {
		return apierr.ErrNotFound
	}
	if err := s.Catalog.SetPluginArchived(ctx, name, archived); err != nil {
		return err
	}
	action := model.AuditPluginUnarchived
	if archived {
		action = model.AuditPluginArchived
	}
	s.record(ctx, actorID, action, entry.Name, entry.Name, "")
	return nil
}

// ListEntries returns the catalog. Archived entries are included only when
// asked for: hiding a retired entry from the person who retired it would leave
// no way to restore it.
func (s *Service) ListEntries(ctx context.Context, includeArchived bool) ([]model.Plugin, error) {
	return s.Catalog.ListPlugins(ctx, includeArchived)
}

// GetEntry returns one catalog entry, or (nil, nil) when there is none.
func (s *Service) GetEntry(ctx context.Context, name string) (*model.Plugin, error) {
	return s.Catalog.GetPlugin(ctx, name)
}

// ListReleases returns every release of one plugin, yanked ones included:
// which to install needs the version arithmetic, and an exact version can still
// be recovered by someone who acknowledges the state.
func (s *Service) ListReleases(ctx context.Context, name string) ([]model.PluginRelease, error) {
	entry, err := s.Catalog.GetPlugin(ctx, name)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, apierr.ErrNotFound
	}
	return s.Catalog.ListPluginReleases(ctx, name)
}

// GetRelease returns one release, or (nil, nil) when there is none.
func (s *Service) GetRelease(ctx context.Context, name, version string) (*model.PluginRelease, error) {
	return s.Catalog.GetPluginRelease(ctx, name, version)
}

// OpenPackage streams one release's bytes.
//
// The stream is handed to the caller rather than read here: a download that
// buffered a package would size the server by its largest plugin.
func (s *Service) OpenPackage(ctx context.Context, release model.PluginRelease) (io.ReadCloser, int64, error) {
	return s.Packages.Open(ctx, release.ObjectKey)
}

// Yank withdraws a release from default selection.
func (s *Service) Yank(ctx context.Context, name, version, actorID, reason string) error {
	release, err := s.Catalog.GetPluginRelease(ctx, name, version)
	if err != nil {
		return err
	}
	if release == nil {
		return apierr.ErrNotFound
	}
	if err := s.Catalog.YankPluginRelease(ctx, name, version, actorID, reason); err != nil {
		return err
	}
	s.record(ctx, actorID, model.AuditPluginYanked, name, releaseTarget(name, version),
		releaseDetail(name, version, release.Digest))
	return nil
}

// PublishInput is one upload.
type PublishInput struct {
	// PluginName is the route's name. The manifest inside the package has to
	// agree with it.
	PluginName string
	// Body is the archive, streamed rather than held.
	Body io.Reader
	// Source is the publisher's claim about the checkout the bytes came from.
	// The server cannot verify it, so it is recorded as a claim beside a digest
	// the server calculated itself.
	Source  model.PluginReleaseSource
	ActorID string
}

// Publish stores one release.
//
// The order matters. Bytes are stored before the release row, so a failure
// between the two leaves an orphan at a content-addressed key rather than a row
// pointing at nothing. An orphan costs disk; a dangling row costs an install.
func (s *Service) Publish(ctx context.Context, in PublishInput) (*model.PluginRelease, error) {
	staged, err := s.stage(in.Body)
	if err != nil {
		return nil, err
	}
	defer staged.cleanup()

	pkg, err := inspect.Dir(os.DirFS(staged.dir))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPackage, err)
	}
	if errs := coreplugin.Errors(pkg.Findings); len(errs) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPackage, errs[0].String())
	}
	if pkg.Manifest.Name != in.PluginName {
		return nil, fmt.Errorf("%w: package names %q, published to %q",
			ErrNameMismatch, pkg.Manifest.Name, in.PluginName)
	}
	// The version comes from the packed manifest rather than from a field the
	// client sent, so the release row and the bytes cannot disagree.
	if pkg.Manifest.Version == "" {
		return nil, fmt.Errorf("%w: the manifest has no version, which publishing requires", ErrInvalidPackage)
	}
	if _, err := coreplugin.ParseVersion(pkg.Manifest.Version); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPackage, err)
	}
	if v := pkg.Manifest.MinBuildmaxVersion; v != "" {
		if _, err := coreplugin.ParseVersion(v); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidPackage, err)
		}
	}

	entry, err := s.ensureEntry(ctx, pkg.Manifest, in.ActorID)
	if err != nil {
		return nil, err
	}
	if entry.Archived() {
		return nil, model.ErrPluginArchived
	}

	key, err := s.Packages.PackageKey(s.KeyPrefix, entry.Name, staged.digest)
	if err != nil {
		return nil, err
	}
	if err := s.storeBytes(ctx, key, staged.path); err != nil {
		return nil, err
	}

	release, err := s.Catalog.CreatePluginRelease(ctx, model.CreatePluginReleaseInput{
		PluginName:         entry.Name,
		Version:            pkg.Manifest.Version,
		MinBuildmaxVersion: pkg.Manifest.MinBuildmaxVersion,
		Digest:             staged.digest,
		ObjectKey:          key,
		SizeBytes:          staged.size,
		Inspection:         toInspection(pkg),
		Source:             in.Source,
		PublishedBy:        in.ActorID,
	})
	if err != nil {
		return nil, err
	}
	s.record(ctx, in.ActorID, model.AuditPluginPublished, entry.Name, releaseTarget(entry.Name, release.Version),
		releaseDetail(entry.Name, release.Version, release.Digest))
	return release, nil
}

// ensureEntry returns the catalog entry, creating it on a first publish.
//
// The design shows publishing as one command against a directory, so requiring
// a separate create would make the documented flow fail on its first use. The
// authority is the same either way — only a System Administrator reaches
// either route — and the trail records the creation as its own event.
func (s *Service) ensureEntry(ctx context.Context, m coreplugin.Manifest, actorID string) (*model.Plugin, error) {
	entry, err := s.Catalog.GetPlugin(ctx, m.Name)
	if err != nil {
		return nil, err
	}
	if entry != nil {
		return entry, nil
	}
	return s.CreateEntry(ctx, CreateEntryInput{
		Name:        m.Name,
		DisplayName: m.DisplayName,
		Description: m.Description,
		ActorID:     actorID,
	})
}

// storeBytes uploads the staged archive.
func (s *Service) storeBytes(ctx context.Context, key, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return s.Packages.Put(ctx, key, f)
}

// toInspection reduces a package report to what a catalog record keeps.
func toInspection(pkg inspect.Package) model.PluginInspection {
	out := model.PluginInspection{
		Skills:      pkg.Skills,
		Subagents:   pkg.Subagents,
		MCP:         pkg.MCP,
		Hooks:       pkg.Hooks,
		EnvRefs:     pkg.EnvRefs,
		PluginPaths: pkg.PluginPaths,
	}
	for _, f := range pkg.Findings {
		// Errors never reach here — publishing already refused them — so what
		// is left is what the publisher chose to accept, and an installer
		// should see the same list they did.
		out.Warnings = append(out.Warnings, f.String())
	}
	return out
}

func releaseDetail(name, version, digest string) string {
	short := digest
	if hexPart, ok := strings.CutPrefix(digest, archive.DigestPrefix); ok && len(hexPart) > digestPrefixLen {
		short = archive.DigestPrefix + hexPart[:digestPrefixLen]
	}
	return fmt.Sprintf("%s@%s %s", name, version, short)
}

// releaseTarget names one release for the audit trail. A release has no handle
// of its own: name plus version is what every route and every install addresses
// it by, and it is immutable, which is what an audit record needs.
func releaseTarget(name, version string) string { return name + "@" + version }

func (s *Service) record(ctx context.Context, actorID, action, name, targetID, detail string) {
	if detail == "" {
		detail = name
	}
	s.Audit.Record(ctx, model.AuditEvent{
		ActorType:  model.AuditActorUser,
		ActorID:    actorID,
		Action:     action,
		TargetType: "plugin",
		TargetID:   targetID,
		Detail:     detail,
	})
}

// stagedPackage is one upload on local disk, already hashed and unpacked.
type stagedPackage struct {
	path   string
	dir    string
	digest string
	size   int64
}

func (p stagedPackage) cleanup() {
	_ = os.Remove(p.path)
	_ = os.RemoveAll(p.dir)
}

// stage writes the upload to disk while hashing it, then extracts it.
//
// The bytes land on disk first because three things need them — the digest, the
// inspection, and the object store — and a server that held a package in memory
// to serve all three would be sized by its largest plugin rather than by its
// traffic.
func (s *Service) stage(body io.Reader) (stagedPackage, error) {
	lim := s.Limits
	f, err := os.CreateTemp("", "buildmax-plugin-*.tar.gz")
	if err != nil {
		return stagedPackage{}, err
	}
	staged := stagedPackage{path: f.Name()}

	hash := sha256.New()
	// One byte past the allowance separates "exactly at the limit" from "over
	// it", and stops a large upload before it fills a disk.
	maxCompressed := archive.ResolveLimits(lim).MaxCompressedBytes
	size, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(body, maxCompressed+1))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		staged.cleanup()
		return stagedPackage{}, err
	}
	if size > maxCompressed {
		staged.cleanup()
		return stagedPackage{}, fmt.Errorf("%w: %s", ErrInvalidPackage, archive.ErrTooLarge)
	}
	staged.size = size
	staged.digest = archive.DigestPrefix + hex.EncodeToString(hash.Sum(nil))

	dir, err := os.MkdirTemp("", "buildmax-plugin-extract-")
	if err != nil {
		staged.cleanup()
		return stagedPackage{}, err
	}
	staged.dir = dir

	archiveFile, err := os.Open(staged.path)
	if err != nil {
		staged.cleanup()
		return stagedPackage{}, err
	}
	defer archiveFile.Close()
	if _, err := archive.Extract(archiveFile, dir, lim); err != nil {
		staged.cleanup()
		return stagedPackage{}, fmt.Errorf("%w: %s", ErrInvalidPackage, err)
	}
	return staged, nil
}
