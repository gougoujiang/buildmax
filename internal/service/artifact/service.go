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
	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	"io"
	"mime"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
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
	// ErrStorageQuota is the team's allowance, not this file's size. It is a
	// quota kind so it answers 429 like the run and token limits rather than
	// 400 like a file this deployment would never accept at any allowance.
	ErrStorageQuota = apierr.New(apierr.KindQuotaExceeded, "this space has no room for more artifacts")
)

// StorageAdmitter decides whether a team may hold more bytes.
//
// Declared here rather than imported from the quota service so this package
// depends on the one question it asks. A nil admitter admits everything, which
// is what a deployment with no quota service has.
type StorageAdmitter interface {
	// CheckStorage reports whether the team may hold addBytes more, and why
	// not when it may not.
	CheckStorage(ctx context.Context, teamID string, addBytes int64) (bool, string, error)
}

// DefaultMaxFileBytes caps one artifact when a deployment sets no limit.
//
// It bounds one request. What a team may hold in total is the quota tier's
// max_storage_bytes, checked through StorageAdmitter: the two answer different
// questions, and neither substitutes for the other — a thousand small files
// pass this cap and can still exhaust an allowance.
const DefaultMaxFileBytes int64 = 100 << 20

// FallbackMediaType is what an unrecognised extension gets. Unknown types
// download as attachments, so guessing generously would only mislabel them.
const FallbackMediaType = "application/octet-stream"

const maxFilenameLen = 255

// Service creates, reads, lists, and tombstones artifacts.
type Service struct {
	Artifacts coreartifact.Store
	Storage   ContentStore
	// Audit records that a file entered or left a team's keeping. Nil records
	// nothing, which is what a deployment without a database has.
	Audit *audit.Recorder
	// MaxFileBytes caps one artifact. Zero means DefaultMaxFileBytes.
	MaxFileBytes int64
	// Quota decides whether the team may hold more. Nil admits everything.
	Quota StorageAdmitter

	// Shares persists public links. Nil means this deployment cannot create
	// them, which is what a deployment without the share store has.
	Shares coreartifact.ShareStore
	// PublicBaseURL is the externally reachable origin share links are rendered
	// against. Empty refuses share creation rather than emitting a link nobody
	// can open — see docs/design/artifact-public-sharing-and-preview.md §9.
	PublicBaseURL string
	// ShareTTL bounds a link's lifetime (default and maximum). Zero means
	// DefaultShareTTL.
	ShareTTL time.Duration

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
	ExpiresAt     *time.Time
}

// Create stores the content, then records the artifact.
//
// The order is the contract: metadata is committed only once the bytes are
// durable, and every failure path removes whatever was written. A caller that
// gets an error has no artifact, and nothing is left addressable that a record
// does not describe.
func (s *Service) Create(ctx context.Context, in CreateInput) (*coreartifact.Artifact, error) {
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

	// Asked before reading the body, so a team that is already full is refused
	// without first streaming a file to disk. It cannot be the only check: the
	// size is not known until the bytes have gone by, so the exact one happens
	// below and this is the cheap rejection in front of it.
	if err := s.admitStorage(ctx, in.TeamID, 0); err != nil {
		return nil, err
	}

	// Generated here rather than in the store because the content is written to
	// object storage under this ID before the row exists; moving generation down
	// is part of giving the store its collision retry.
	artifactID, err := util.NewPublicID()
	if err != nil {
		return nil, err
	}
	ref := coreartifact.Ref{TeamID: in.TeamID, ArtifactID: artifactID}
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

	// The exact check, now that the size is known. It runs after the object is
	// durable because that is the first moment the size exists; refusing here
	// removes what was written, so an over-quota upload leaves nothing behind.
	if err := s.admitStorage(ctx, in.TeamID, counter.n); err != nil {
		s.discard(ctx, ref)
		return nil, err
	}

	rec, err := s.Artifacts.CreateArtifact(ctx, coreartifact.CreateInput{
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
	s.audit(ctx, rec, coreaudit.ArtifactCreated, rec.SourceType)
	return rec, nil
}

// admitStorage asks whether the team may hold addBytes more.
//
// A quota that cannot be read refuses, matching the run and token limits: the
// alternative is that a deployment whose database is unreachable accepts
// unmetered storage and has no record of having done so.
func (s *Service) admitStorage(ctx context.Context, teamID string, addBytes int64) error {
	if s.Quota == nil {
		return nil
	}
	allowed, reason, err := s.Quota.CheckStorage(ctx, teamID, addBytes)
	if err != nil {
		return err
	}
	if !allowed {
		if reason == "" {
			return ErrStorageQuota
		}
		return apierr.Detail(ErrStorageQuota, "%s", reason)
	}
	return nil
}

// discard removes content that no committed record describes.
//
// A failure to remove is not reported to the caller: the caller's operation
// already failed, and the object is unreferenced either way. What it leaves is
// an orphan for a reconciler, which is strictly better than a record pointing
// at bytes that are not there.
func (s *Service) discard(ctx context.Context, ref coreartifact.Ref) {
	_ = s.Storage.RemoveArtifact(ctx, ref)
}

// Get returns a live artifact. A tombstoned one is reported as absent, so
// deletion takes effect here rather than at each caller.
func (s *Service) Get(ctx context.Context, artifactID string) (*coreartifact.Artifact, error) {
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
func (s *Service) Open(ctx context.Context, rec *coreartifact.Artifact) (io.ReadCloser, error) {
	if !s.Available() {
		return nil, ErrNotConfigured
	}
	if rec == nil {
		return nil, ErrNotFound
	}
	body, err := s.Storage.OpenArtifact(ctx, coreartifact.Ref{TeamID: rec.TeamID, ArtifactID: rec.ID})
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return body, nil
}

// List returns a team's live artifacts, newest first, with the total.
func (s *Service) List(ctx context.Context, teamID string, limit, offset int) ([]coreartifact.Artifact, int, error) {
	if !s.Available() {
		return nil, 0, ErrNotConfigured
	}
	return s.Artifacts.ListArtifactsByTeam(ctx, teamID, limit, offset)
}

// ListBySource returns what the given operations published, keyed by source ID.
// A work object uses it to show what its runs produced without becoming their
// owner.
func (s *Service) ListBySource(ctx context.Context, sourceIDs []string) (map[string][]coreartifact.Artifact, error) {
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
func (s *Service) Delete(ctx context.Context, rec *coreartifact.Artifact, actorType, actorID string) error {
	if !s.Available() {
		return ErrNotConfigured
	}
	if rec == nil {
		return ErrNotFound
	}
	changed, err := s.Artifacts.SoftDeleteArtifact(ctx, rec.ID, s.now().UTC())
	if err != nil {
		return err
	}
	if !changed {
		return ErrNotFound
	}
	deleted := *rec
	deleted.CreatedByType = actorType
	deleted.CreatedByID = actorID
	s.audit(ctx, &deleted, coreaudit.ArtifactDeleted, "")
	return nil
}

// audit records a metadata-only event. The detail is the source type or empty:
// a filename or a caller's title is content, and content does not go in the
// trail.
func (s *Service) audit(ctx context.Context, rec *coreartifact.Artifact, action, detail string) {
	if s.Audit == nil || rec == nil {
		return
	}
	s.Audit.Record(ctx, coreaudit.Event{
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
	case coreartifact.CreatorUser:
		return coreaudit.ActorUser
	case coreartifact.CreatorAgent, coreartifact.CreatorWorker:
		return coreaudit.ActorWorker
	default:
		return coreaudit.ActorSystem
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
	ext := strings.ToLower(filepath.Ext(filename))
	// Pinned before the system table: preview depends on these being recognised,
	// and Go's mime does not register Markdown at all, while the system table is
	// a deployment variable we do not want the render decision to ride on. HTML
	// is pinned too so an artifact previews under the same type on every host.
	if t, ok := pinnedMediaTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return FallbackMediaType
}

// pinnedMediaTypes fixes the types the preview surface keys on, independent of
// the host's mime database.
var pinnedMediaTypes = map[string]string{
	".md":       "text/markdown; charset=utf-8",
	".markdown": "text/markdown; charset=utf-8",
	".html":     "text/html; charset=utf-8",
	".htm":      "text/html; charset=utf-8",
}

// countingWriter counts bytes and keeps none of them.
type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}
