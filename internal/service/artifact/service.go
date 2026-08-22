// Package artifact keeps durable files on a team's behalf.
//
// It is the whole artifact capability, and it is deliberately ignorant of what
// produced a file: an agent, a worker run, and a person uploading reach the
// same Create with different provenance strings. Nothing here imports task
// runs, issues, or conversations, which is what stops an artifact from
// acquiring a parent. See docs/design/unified-artifacts.md.
package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/util"
)

var (
	ErrNotConfigured = apierr.New(apierr.KindNotConfigured, "artifacts are not configured")
	ErrNotFound      = apierr.New(apierr.KindNotFound, "artifact not found")
	ErrNoFilename    = apierr.New(apierr.KindInvalid, "a filename is required")
	ErrNoTeam        = apierr.New(apierr.KindInvalid, "a team is required")
	ErrEmptyContent  = apierr.New(apierr.KindInvalid, "the file is empty")
	ErrTooLarge      = apierr.New(apierr.KindInvalid, "the file is larger than this deployment accepts")
)

// DefaultMaxFileBytes caps one artifact when a deployment sets no limit.
//
// A cap is the whole of the first slice's admission control. A team storage
// total is a stock, and the existing quota model measures rates in a window
// (model.QuotaTier), so counting bytes held is a new dimension rather than a
// value to slot in; it waits for its own decision. What this cap already buys
// is the property that matters most here — one request cannot cost the
// deployment an unbounded amount of disk.
const DefaultMaxFileBytes int64 = 100 << 20

// FallbackMediaType is what an unrecognised extension gets. Unknown types
// download as attachments, so guessing generously would only mislabel them.
const FallbackMediaType = "application/octet-stream"

const maxFilenameLen = 255

// Service creates, reads, lists, and tombstones artifacts.
type Service struct {
	Artifacts model.ArtifactStore
	Storage   blob.ArtifactStorage
	// Audit records that a file entered or left a team's keeping. Nil records
	// nothing, which is what a deployment without a database has.
	Audit *audit.Recorder
	// MaxFileBytes caps one artifact. Zero means DefaultMaxFileBytes.
	MaxFileBytes int64

	clock func() time.Time
}

func (s *Service) now() time.Time {
	if s != nil && s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

func (s *Service) maxFileBytes() int64 {
	if s != nil && s.MaxFileBytes > 0 {
		return s.MaxFileBytes
	}
	return DefaultMaxFileBytes
}

// MaxBytes reports the largest file this deployment accepts, so a transport
// can refuse an oversized body before reading it rather than after.
func (s *Service) MaxBytes() int64 { return s.maxFileBytes() }

// Available reports whether this deployment can hold artifacts at all. Both
// halves are required: metadata without content, or content without a record,
// is not an artifact.
func (s *Service) Available() bool {
	return s != nil && s.Artifacts != nil && s.Storage != nil
}

// CreateInput describes one file to keep and who produced it.
type CreateInput struct {
	TeamID string
	// Filename is the content's name. Any directory part is discarded: an
	// artifact is one file, and a caller-supplied path is not a place the
	// server should be persuaded to reason about.
	Filename string
	Title    string
	// SourceType and SourceID are provenance, not ownership — see
	// model.ArtifactSource*.
	SourceType string
	SourceID   string
	// CreatedByType names the kind of actor; CreatedByID is empty for
	// automated work rather than carrying an invented user.
	CreatedByType string
	CreatedByID   string
	Content       io.Reader
	ExpiresAt     *int64
}

// Create stores the content, then records the artifact.
//
// The order is the contract: metadata is committed only once the bytes are
// durable, and every failure path removes whatever was written. A caller that
// gets an error has no artifact, and nothing is left addressable that a record
// does not describe.
func (s *Service) Create(ctx context.Context, in CreateInput) (*model.Artifact, error) {
	if !s.Available() {
		return nil, ErrNotConfigured
	}
	filename, err := cleanFilename(in.Filename)
	if err != nil {
		return nil, err
	}
	if in.TeamID == "" {
		return nil, ErrNoTeam
	}
	if in.Content == nil {
		return nil, ErrEmptyContent
	}

	// Generated here rather than in the store because the content is written to
	// object storage under this ID before the row exists; moving generation down
	// is part of giving the store its collision retry.
	artifactID, err := util.NewPublicID()
	if err != nil {
		return nil, err
	}
	ref := blob.ArtifactRef{TeamID: in.TeamID, ArtifactID: artifactID}
	limit := s.maxFileBytes()

	// Read one byte past the limit so an oversized upload is detected rather
	// than silently truncated. Size and digest are measured from the bytes that
	// went by, never from a header the uploader chose.
	hash := sha256.New()
	counter := &countingWriter{}
	body := io.TeeReader(io.LimitReader(in.Content, limit+1), io.MultiWriter(hash, counter))

	storageKey, err := s.Storage.PutArtifact(ctx, ref, body)
	if err != nil {
		s.discard(ctx, ref)
		return nil, err
	}
	switch {
	case counter.n == 0:
		s.discard(ctx, ref)
		return nil, ErrEmptyContent
	case counter.n > limit:
		s.discard(ctx, ref)
		return nil, apierr.Detail(ErrTooLarge, "limit is %d bytes", limit)
	}

	rec, err := s.Artifacts.CreateArtifact(ctx, model.CreateArtifactInput{
		TeamID:        in.TeamID,
		ArtifactID:    artifactID,
		Filename:      filename,
		MediaType:     mediaTypeFor(filename),
		SizeBytes:     counter.n,
		SHA256:        hex.EncodeToString(hash.Sum(nil)),
		StorageKey:    storageKey,
		CreatedByType: in.CreatedByType,
		CreatedByID:   in.CreatedByID,
		SourceType:    in.SourceType,
		SourceID:      in.SourceID,
		Title:         in.Title,
		ExpiresAt:     in.ExpiresAt,
	})
	if err != nil {
		s.discard(ctx, ref)
		return nil, err
	}
	s.audit(ctx, rec, model.AuditArtifactCreated, rec.SourceType)
	return rec, nil
}

// discard removes content that no committed record describes.
//
// A failure to remove is not reported to the caller: the caller's operation
// already failed, and the object is unreferenced either way. What it leaves is
// an orphan for a reconciler, which is strictly better than a record pointing
// at bytes that are not there.
func (s *Service) discard(ctx context.Context, ref blob.ArtifactRef) {
	_ = s.Storage.RemoveArtifact(ctx, ref)
}

// Get returns a live artifact. A tombstoned one is reported as absent, so
// deletion takes effect here rather than at each caller.
func (s *Service) Get(ctx context.Context, artifactID string) (*model.Artifact, error) {
	if !s.Available() {
		return nil, ErrNotConfigured
	}
	rec, err := s.Artifacts.GetArtifact(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	if rec == nil || rec.Deleted() {
		return nil, ErrNotFound
	}
	return rec, nil
}

// Open returns the artifact's content stream, which the caller closes.
func (s *Service) Open(ctx context.Context, rec *model.Artifact) (io.ReadCloser, error) {
	if !s.Available() {
		return nil, ErrNotConfigured
	}
	if rec == nil {
		return nil, ErrNotFound
	}
	body, err := s.Storage.OpenArtifact(ctx, blob.ArtifactRef{TeamID: rec.TeamID, ArtifactID: rec.ID})
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return body, nil
}

// List returns a team's live artifacts, newest first, with the total.
func (s *Service) List(ctx context.Context, teamID string, limit, offset int) ([]model.Artifact, int, error) {
	if !s.Available() {
		return nil, 0, ErrNotConfigured
	}
	return s.Artifacts.ListArtifactsByTeam(ctx, teamID, limit, offset)
}

// ListBySource returns what the given operations published, keyed by source ID.
// A work object uses it to show what its runs produced without becoming their
// owner.
func (s *Service) ListBySource(ctx context.Context, sourceIDs []string) (map[string][]model.Artifact, error) {
	if !s.Available() {
		return nil, ErrNotConfigured
	}
	return s.Artifacts.ListArtifactsBySource(ctx, sourceIDs)
}

// Delete tombstones the artifact.
//
// Content removal is deliberately not done here. The tombstone is what makes
// deletion take effect at the authorization boundary immediately, which is the
// property that matters; reclaiming the object is retention's job and can be
// slower than the request that asked for it.
func (s *Service) Delete(ctx context.Context, rec *model.Artifact, actorType, actorID string) error {
	if !s.Available() {
		return ErrNotConfigured
	}
	if rec == nil {
		return ErrNotFound
	}
	changed, err := s.Artifacts.SoftDeleteArtifact(ctx, rec.ID, s.now().Unix())
	if err != nil {
		return err
	}
	if !changed {
		return ErrNotFound
	}
	deleted := *rec
	deleted.CreatedByType = actorType
	deleted.CreatedByID = actorID
	s.audit(ctx, &deleted, model.AuditArtifactDeleted, "")
	return nil
}

// audit records a metadata-only event. The detail is the source type or empty:
// a filename or a caller's title is content, and content does not go in the
// trail.
func (s *Service) audit(ctx context.Context, rec *model.Artifact, action, detail string) {
	if s.Audit == nil || rec == nil {
		return
	}
	s.Audit.Record(ctx, model.AuditEvent{
		TeamID:     rec.TeamID,
		ActorType:  auditActorFor(rec.CreatedByType),
		ActorID:    rec.CreatedByID,
		Action:     action,
		TargetType: "artifact",
		TargetID:   rec.ID,
		Detail:     detail,
	})
}

func auditActorFor(createdByType string) string {
	switch createdByType {
	case model.ArtifactCreatorUser:
		return model.AuditActorUser
	case model.ArtifactCreatorAgent, model.ArtifactCreatorWorker:
		return model.AuditActorWorker
	default:
		return model.AuditActorSystem
	}
}

// cleanFilename reduces a caller's name to one path element.
func cleanFilename(name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	name = path.Base(name)
	if name == "" || name == "." || name == ".." || name == "/" {
		return "", ErrNoFilename
	}
	if len(name) > maxFilenameLen {
		name = name[:maxFilenameLen]
	}
	return name, nil
}

// mediaTypeFor decides the type from the stored filename rather than from
// anything the uploader declared. A caller's Content-Type is a claim about
// bytes the server is about to serve back to a browser, so it is not the basis
// for the header that decides how the browser treats them.
func mediaTypeFor(filename string) string {
	if t := mime.TypeByExtension(filepath.Ext(filename)); t != "" {
		return t
	}
	return FallbackMediaType
}

// countingWriter counts bytes and keeps none of them.
type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}
