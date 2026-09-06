package objectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// checkpointStore is the surface both backends implement, so one test exercises
// each identically. (fakeS3 is defined in plugin_test.go and reused here.)
type checkpointStore interface {
	Put(ctx context.Context, spaceID, sha256hex string, src io.Reader) (string, error)
	Open(ctx context.Context, storageKey string) (io.ReadCloser, int64, error)
	Exists(ctx context.Context, spaceID, sha256hex string) (bool, error)
	Delete(ctx context.Context, storageKey string) error
	ListBlobs(ctx context.Context) ([]ObjectInfo, error)
}

const validDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestCheckpointStoresRoundTrip(t *testing.T) {
	ctx := t.Context()
	cases := map[string]checkpointStore{
		"localfs": NewLocalFSCheckpointStore(t.TempDir()),
		"s3":      NewS3CheckpointStore(&fakeS3{objects: map[string][]byte{}}, "bucket", "pre"),
	}
	for name, store := range cases {
		t.Run(name, func(t *testing.T) {
			payload := []byte("a checkpoint archive")
			key, err := store.Put(ctx, "sp_space1", validDigest, bytes.NewReader(payload))
			if err != nil {
				t.Fatalf("Put: %v", err)
			}
			if !strings.Contains(key, "sp_space1/workspace/blobs/sha256/"+validDigest) {
				t.Errorf("storage key = %q, want it content-addressed under the space", key)
			}

			ok, err := store.Exists(ctx, "sp_space1", validDigest)
			if err != nil || !ok {
				t.Fatalf("Exists after Put = %v, %v; want true", ok, err)
			}

			rc, size, err := store.Open(ctx, key)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			got, _ := io.ReadAll(rc)
			_ = rc.Close()
			if !bytes.Equal(got, payload) {
				t.Errorf("Open bytes = %q, want %q", got, payload)
			}
			if size != int64(len(payload)) {
				t.Errorf("Open size = %d, want %d", size, len(payload))
			}

			if err := store.Delete(ctx, key); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			ok, err = store.Exists(ctx, "sp_space1", validDigest)
			if err != nil || ok {
				t.Errorf("Exists after Delete = %v, %v; want false", ok, err)
			}
			// Delete is safe to repeat.
			if err := store.Delete(ctx, key); err != nil {
				t.Errorf("second Delete: %v", err)
			}
		})
	}
}

// TestCheckpointStoresListBlobs pins that ListBlobs returns exactly the
// content-addressed payloads, keyed as they were stored (so the sweep can match
// them against the database), and skips anything sharing the store that is not a
// blob.
func TestCheckpointStoresListBlobs(t *testing.T) {
	ctx := t.Context()
	digestB := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	newStores := func(t *testing.T) (checkpointStore, string) {
		fake := &fakeS3{objects: map[string][]byte{}}
		// An object under the same bucket/prefix that is not a checkpoint payload:
		// ListBlobs must not return it.
		fake.objects["pre/sp_space1/artifacts/a1/file.txt"] = []byte("not a checkpoint")
		return NewS3CheckpointStore(fake, "bucket", "pre"), "s3"
	}
	for _, mk := range []func(*testing.T) (checkpointStore, string){
		newStores,
		func(t *testing.T) (checkpointStore, string) { return NewLocalFSCheckpointStore(t.TempDir()), "localfs" },
	} {
		store, name := mk(t)
		t.Run(name, func(t *testing.T) {
			k1, err := store.Put(ctx, "sp_space1", validDigest, bytes.NewReader([]byte("one")))
			if err != nil {
				t.Fatal(err)
			}
			k2, err := store.Put(ctx, "sp_space2", digestB, bytes.NewReader([]byte("two")))
			if err != nil {
				t.Fatal(err)
			}
			blobs, err := store.ListBlobs(ctx)
			if err != nil {
				t.Fatalf("ListBlobs: %v", err)
			}
			got := map[string]bool{}
			for _, b := range blobs {
				got[b.Key] = true
			}
			if len(got) != 2 || !got[k1] || !got[k2] {
				t.Fatalf("ListBlobs keys = %v, want exactly {%q, %q}", got, k1, k2)
			}
		})
	}
}

func TestCheckpointStoreRejectsABadDigestOrSpace(t *testing.T) {
	ctx := t.Context()
	store := NewLocalFSCheckpointStore(t.TempDir())
	bad := []struct{ space, digest string }{
		{"sp_space1", "tooshort"},
		{"sp_space1", strings.Repeat("g", 64)}, // 'g' is not hex
		{"sp_space1", validDigest + "extra"},
		{"../evil", validDigest},
		{"sp/slash", validDigest},
	}
	for _, c := range bad {
		if _, err := store.Put(ctx, c.space, c.digest, bytes.NewReader([]byte("x"))); !errors.Is(err, ErrInvalidCheckpointDigest) {
			t.Errorf("Put(%q,%q) error = %v, want ErrInvalidCheckpointDigest", c.space, c.digest, err)
		}
	}
}
