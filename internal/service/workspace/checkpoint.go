// Package workspace owns the cross-storage commit of a Task workspace
// checkpoint: it validates a captured payload's descriptor, confirms the
// immutable bytes are already durable in the payload store, and only then
// records the authoritative metadata pointer through the metadata store. This
// is the ordering the commit protocol requires — bytes first, pointer second —
// so a recorded checkpoint never dangles over missing bytes. See
// docs/design/task-workspace-checkpoints.md §8 and §10.
package workspace

import (
	"context"
	"errors"
	"fmt"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
)

// MetadataStore records the authoritative checkpoint pointer atomically with
// the source run and, for a head-advancing kind, the Task head. Its
// authoritative implementation is the db Store.
type MetadataStore interface {
	FinalizeWorkspaceCheckpoint(ctx context.Context, in coretask.FinalizeCheckpointInput) (*coretask.WorkspaceCheckpoint, error)
}

// PayloadStore holds the immutable checkpoint bytes, content-addressed by the
// Space and the payload's SHA-256. The service only reads it here — Exists is
// the pre-commit check that the bytes the descriptor points at are durable.
type PayloadStore interface {
	Exists(ctx context.Context, spaceID, sha256hex string) (bool, error)
}

// ErrPayloadMissing is returned when a descriptor is finalized before its bytes
// reached the payload store. The metadata pointer must never be recorded first;
// this is the guard that keeps the ordering true even when a caller gets it
// wrong.
var ErrPayloadMissing = errors.New("workspace: checkpoint payload bytes are not in the store")

// ErrInvalidDescriptor is a structurally invalid payload descriptor — an
// unsupported format, a malformed digest, an empty storage key, or a
// non-positive size. It is a programming error at the call site, not a
// user-facing condition.
var ErrInvalidDescriptor = errors.New("workspace: invalid checkpoint descriptor")

// PayloadDescriptor is what a capture produced: the format and content digest
// of the bytes, where they live, and the counters the archive reported. The
// service trusts these numbers for the metadata record; it does not re-read the
// object to recount.
type PayloadDescriptor struct {
	Format            string
	SHA256            string
	StorageKey        string
	SizeBytes         int64
	UncompressedBytes int64
	EntryCount        int64
}

// RecordBaseInput records the seed checkpoint that gives a Task its first
// workspace head — the materialized starting point a first run restores from.
type RecordBaseInput struct {
	SpaceID         string
	TaskID          string
	SourceTaskRunID string
	Payload         PayloadDescriptor
}

// FinalizeResultInput records the checkpoint a run produced from its workspace.
// Succeeded distinguishes the two result kinds: a successful result advances the
// Task head from the run's base; a partial preserves a failed run's work
// without moving the head. Base is the checkpoint the run started from, carried
// so the store can verify a linear head advance.
type FinalizeResultInput struct {
	SpaceID          string
	TaskID           string
	SourceTaskRunID  string
	BaseCheckpointID *string
	Succeeded        bool
	Payload          PayloadDescriptor
}

// Service is the checkpoint finalizer. It is stateless beyond its two stores.
type Service struct {
	metadata MetadataStore
	payloads PayloadStore
}

// New builds the finalizer over its metadata and payload stores. Both are
// required; a deployment without checkpoint storage does not construct one.
func New(metadata MetadataStore, payloads PayloadStore) *Service {
	return &Service{metadata: metadata, payloads: payloads}
}

// RecordBase validates a seed payload and records it as the Task's base
// checkpoint. A seed has no base of its own and advances a head that has none.
func (s *Service) RecordBase(ctx context.Context, in RecordBaseInput) (*coretask.WorkspaceCheckpoint, error) {
	return s.finalize(ctx, coretask.FinalizeCheckpointInput{
		SpaceID:           in.SpaceID,
		TaskID:            in.TaskID,
		SourceTaskRunID:   in.SourceTaskRunID,
		Kind:              coretask.CheckpointKindSeed,
		PayloadFormat:     in.Payload.Format,
		PayloadSHA256:     in.Payload.SHA256,
		StorageKey:        in.Payload.StorageKey,
		SizeBytes:         in.Payload.SizeBytes,
		UncompressedBytes: in.Payload.UncompressedBytes,
		EntryCount:        in.Payload.EntryCount,
	})
}

// FinalizeResult validates a run's result payload and records it. A successful
// run's checkpoint advances the head from BaseCheckpointID; a failed run's
// partial preserves the work and leaves the head where it is.
func (s *Service) FinalizeResult(ctx context.Context, in FinalizeResultInput) (*coretask.WorkspaceCheckpoint, error) {
	kind := coretask.CheckpointKindPartial
	if in.Succeeded {
		kind = coretask.CheckpointKindSuccessful
	}
	return s.finalize(ctx, coretask.FinalizeCheckpointInput{
		SpaceID:           in.SpaceID,
		TaskID:            in.TaskID,
		SourceTaskRunID:   in.SourceTaskRunID,
		BaseCheckpointID:  in.BaseCheckpointID,
		Kind:              kind,
		PayloadFormat:     in.Payload.Format,
		PayloadSHA256:     in.Payload.SHA256,
		StorageKey:        in.Payload.StorageKey,
		SizeBytes:         in.Payload.SizeBytes,
		UncompressedBytes: in.Payload.UncompressedBytes,
		EntryCount:        in.Payload.EntryCount,
	})
}

func (s *Service) finalize(ctx context.Context, in coretask.FinalizeCheckpointInput) (*coretask.WorkspaceCheckpoint, error) {
	if err := validateDescriptor(in); err != nil {
		return nil, err
	}
	// Bytes before pointer: refuse to record a checkpoint whose payload is not
	// already durable, so the metadata never points at nothing.
	ok, err := s.payloads.Exists(ctx, in.SpaceID, in.PayloadSHA256)
	if err != nil {
		return nil, fmt.Errorf("workspace: check payload bytes: %w", err)
	}
	if !ok {
		return nil, ErrPayloadMissing
	}
	return s.metadata.FinalizeWorkspaceCheckpoint(ctx, in)
}

func validateDescriptor(in coretask.FinalizeCheckpointInput) error {
	switch {
	case in.SpaceID == "" || in.TaskID == "" || in.SourceTaskRunID == "":
		return fmt.Errorf("%w: missing space, task, or run id", ErrInvalidDescriptor)
	case in.PayloadFormat != coretask.PayloadFormatTarZstV1:
		return fmt.Errorf("%w: unsupported payload format %q", ErrInvalidDescriptor, in.PayloadFormat)
	case !isSHA256Hex(in.PayloadSHA256):
		return fmt.Errorf("%w: payload digest is not 64 lowercase hex characters", ErrInvalidDescriptor)
	case in.StorageKey == "":
		return fmt.Errorf("%w: empty storage key", ErrInvalidDescriptor)
	case in.SizeBytes <= 0:
		return fmt.Errorf("%w: non-positive size", ErrInvalidDescriptor)
	case in.UncompressedBytes < 0 || in.EntryCount < 0:
		return fmt.Errorf("%w: negative counters", ErrInvalidDescriptor)
	}
	return nil
}

func isSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
