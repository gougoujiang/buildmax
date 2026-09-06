package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
)

type fakeMetadata struct {
	got  coretask.FinalizeCheckpointInput
	call int
	out  *coretask.WorkspaceCheckpoint
	err  error
}

func (f *fakeMetadata) FinalizeWorkspaceCheckpoint(_ context.Context, in coretask.FinalizeCheckpointInput) (*coretask.WorkspaceCheckpoint, error) {
	f.got = in
	f.call++
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

type fakePayloads struct {
	exists bool
	err    error
	call   int
}

func (f *fakePayloads) Exists(_ context.Context, _, _ string) (bool, error) {
	f.call++
	return f.exists, f.err
}

func (f *fakePayloads) Key(spaceID, sha256hex string) (string, error) {
	return spaceID + "/workspace/blobs/sha256/" + sha256hex, nil
}

const goodDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func goodDescriptor() PayloadDescriptor {
	return PayloadDescriptor{
		Format:            coretask.PayloadFormatTarZstV1,
		SHA256:            goodDigest,
		SizeBytes:         512,
		UncompressedBytes: 2048,
		EntryCount:        7,
	}
}

func TestRecordBaseFinalizesASeedWhenBytesArePresent(t *testing.T) {
	meta := &fakeMetadata{out: &coretask.WorkspaceCheckpoint{ID: "cp1"}}
	pay := &fakePayloads{exists: true}
	svc := New(meta, pay)

	out, err := svc.RecordBase(context.Background(), RecordBaseInput{
		SpaceID:         "sp1",
		TaskID:          "tk1",
		SourceTaskRunID: "rn1",
		Payload:         goodDescriptor(),
	})
	if err != nil {
		t.Fatalf("RecordBase: %v", err)
	}
	if out.ID != "cp1" {
		t.Fatalf("returned checkpoint = %q, want cp1", out.ID)
	}
	if pay.call != 1 {
		t.Fatalf("payload Exists called %d times, want 1", pay.call)
	}
	if meta.got.Kind != coretask.CheckpointKindSeed {
		t.Fatalf("kind = %q, want seed", meta.got.Kind)
	}
	if meta.got.BaseCheckpointID != nil {
		t.Fatalf("seed carried a base checkpoint: %v", *meta.got.BaseCheckpointID)
	}
	if meta.got.PayloadSHA256 != goodDigest || meta.got.StorageKey == "" {
		t.Fatalf("descriptor not forwarded: %+v", meta.got)
	}
}

func TestFinalizeResultKindFollowsSuccess(t *testing.T) {
	base := "cpbase"
	for _, tc := range []struct {
		name      string
		succeeded bool
		want      coretask.CheckpointKind
	}{
		{"success advances", true, coretask.CheckpointKindSuccessful},
		{"failure is partial", false, coretask.CheckpointKindPartial},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta := &fakeMetadata{out: &coretask.WorkspaceCheckpoint{ID: "cp"}}
			svc := New(meta, &fakePayloads{exists: true})
			if _, err := svc.FinalizeResult(context.Background(), FinalizeResultInput{
				SpaceID:          "sp1",
				TaskID:           "tk1",
				SourceTaskRunID:  "rn1",
				BaseCheckpointID: &base,
				Succeeded:        tc.succeeded,
				Payload:          goodDescriptor(),
			}); err != nil {
				t.Fatalf("FinalizeResult: %v", err)
			}
			if meta.got.Kind != tc.want {
				t.Fatalf("kind = %q, want %q", meta.got.Kind, tc.want)
			}
			if meta.got.BaseCheckpointID == nil || *meta.got.BaseCheckpointID != base {
				t.Fatalf("base checkpoint not forwarded: %+v", meta.got.BaseCheckpointID)
			}
		})
	}
}

func TestFinalizeRefusesWhenPayloadBytesAreMissing(t *testing.T) {
	meta := &fakeMetadata{}
	pay := &fakePayloads{exists: false}
	svc := New(meta, pay)

	_, err := svc.RecordBase(context.Background(), RecordBaseInput{
		SpaceID: "sp1", TaskID: "tk1", SourceTaskRunID: "rn1", Payload: goodDescriptor(),
	})
	if !errors.Is(err, ErrPayloadMissing) {
		t.Fatalf("err = %v, want ErrPayloadMissing", err)
	}
	if meta.call != 0 {
		t.Fatalf("metadata was recorded despite missing bytes")
	}
}

func TestFinalizePropagatesAStoreError(t *testing.T) {
	wantExists := errors.New("boom exists")
	if _, err := New(&fakeMetadata{}, &fakePayloads{err: wantExists}).RecordBase(
		context.Background(), RecordBaseInput{SpaceID: "s", TaskID: "t", SourceTaskRunID: "r", Payload: goodDescriptor()},
	); !errors.Is(err, wantExists) {
		t.Fatalf("Exists error not propagated: %v", err)
	}

	wantMeta := errors.New("boom finalize")
	meta := &fakeMetadata{err: wantMeta}
	if _, err := New(meta, &fakePayloads{exists: true}).RecordBase(
		context.Background(), RecordBaseInput{SpaceID: "s", TaskID: "t", SourceTaskRunID: "r", Payload: goodDescriptor()},
	); !errors.Is(err, wantMeta) {
		t.Fatalf("metadata error not propagated: %v", err)
	}
}

func TestFinalizeRejectsBadDescriptors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mutit func(*PayloadDescriptor)
		ids   [3]string // space, task, run; empty string keeps the good default
	}{
		{"bad format", func(d *PayloadDescriptor) { d.Format = "zip" }, [3]string{}},
		{"short digest", func(d *PayloadDescriptor) { d.SHA256 = "abc" }, [3]string{}},
		{"upper digest", func(d *PayloadDescriptor) { d.SHA256 = strings.ToUpper(goodDigest) }, [3]string{}},
		{"zero size", func(d *PayloadDescriptor) { d.SizeBytes = 0 }, [3]string{}},
		{"negative entries", func(d *PayloadDescriptor) { d.EntryCount = -1 }, [3]string{}},
		{"missing space", nil, [3]string{"", "tk1", "rn1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := goodDescriptor()
			if tc.mutit != nil {
				tc.mutit(&d)
			}
			sp, tk, rn := "sp1", "tk1", "rn1"
			if tc.ids != ([3]string{}) {
				sp, tk, rn = tc.ids[0], tc.ids[1], tc.ids[2]
			}
			meta := &fakeMetadata{}
			pay := &fakePayloads{exists: true}
			_, err := New(meta, pay).RecordBase(context.Background(), RecordBaseInput{
				SpaceID: sp, TaskID: tk, SourceTaskRunID: rn, Payload: d,
			})
			if !errors.Is(err, ErrInvalidDescriptor) {
				t.Fatalf("err = %v, want ErrInvalidDescriptor", err)
			}
			// Validation runs before any store call.
			if pay.call != 0 || meta.call != 0 {
				t.Fatalf("stores touched on an invalid descriptor: exists=%d finalize=%d", pay.call, meta.call)
			}
		})
	}
}
