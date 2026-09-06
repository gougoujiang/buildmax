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
