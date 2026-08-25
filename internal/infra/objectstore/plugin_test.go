package objectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testDigest is the labelled form the archive package produces: sha256 and 64
// lowercase hex characters.
var testDigest = "sha256:" + strings.Repeat("ab12cd34", 8)

func TestPluginPackageKey(t *testing.T) {
	got, err := PluginPackageKey("bm", "code-review", testDigest)
	if err != nil {
		t.Fatal(err)
	}
	want := "bm/plugins/code-review/sha256-" + strings.TrimPrefix(testDigest, "sha256:") + ".tar.gz"
	if got != want {
		t.Errorf("key = %q, want %q", got, want)
	}
	// A colon is not a filename anywhere useful.
	if strings.Contains(got, ":") {
		t.Errorf("key carries the label separator: %q", got)
	}

	// The key is derived from the content: two versions of identical bytes
	// resolve to one object, and different bytes can never collide.
	same, _ := PluginPackageKey("bm", "code-review", testDigest)
	if same != got {
		t.Error("the same digest produced two keys")
	}
	other, err := PluginPackageKey("bm", "code-review", strings.Replace(testDigest, "ab12", "ff99", 1))
	if err != nil {
		t.Fatal(err)
	}
	if other == got {
		t.Error("different digests produced one key")
	}
}

func TestPluginPackageKeyRejections(t *testing.T) {
	tests := []struct {
		name   string
		plugin string
		digest string
	}{
		{"traversal in name", "../escape", testDigest},
		{"nested name", "team/code-review", testDigest},
		{"empty name", "", testDigest},
		{"unlabelled digest", "code-review", strings.TrimPrefix(testDigest, "sha256:")},
		{"wrong algorithm", "code-review", "md5:abc"},
		{"short digest", "code-review", "sha256:abc"},
		{"uppercase digest", "code-review", "sha256:" + strings.Repeat("AB12CD34", 8)},
		{"non-hex digest", "code-review", "sha256:" + strings.Repeat("z", 64)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := PluginPackageKey("bm", tc.plugin, tc.digest); err == nil {
				t.Error("expected a rejection")
			}
		})
	}
}

func TestLocalFSPluginPackageRoundTrip(t *testing.T) {
	root := t.TempDir()
	s := NewLocalFSPluginPackageStorage(root)
	ctx := context.Background()
	key, err := PluginPackageKey("", "code-review", testDigest)
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := s.Exists(ctx, key); err != nil || ok {
		t.Errorf("Exists before Put = %v, %v; want false, nil", ok, err)
	}
	if _, _, err := s.Open(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Errorf("Open before Put: err = %v, want ErrNotFound", err)
	}

	body := []byte("package bytes")
	if err := s.Put(ctx, key, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	ok, err := s.Exists(ctx, key)
	if err != nil || !ok {
		t.Errorf("Exists after Put = %v, %v", ok, err)
	}

	rc, size, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if size != int64(len(body)) {
		t.Errorf("size = %d, want %d", size, len(body))
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("read %q, want %q", got, body)
	}
}

// An upload cut halfway would otherwise leave a short file at a
// content-addressed key: a package whose name says what it holds and whose
// bytes no longer do.
func TestLocalFSPluginPackagePutIsAtomic(t *testing.T) {
	root := t.TempDir()
	s := NewLocalFSPluginPackageStorage(root)
	ctx := context.Background()
	key, _ := PluginPackageKey("", "code-review", testDigest)

	err := s.Put(ctx, key, io.MultiReader(strings.NewReader("half"), failingReader{}))
	if err == nil {
		t.Fatal("a failed upload should be reported")
	}
	if ok, existsErr := s.Exists(ctx, key); existsErr != nil || ok {
		t.Errorf("a failed upload left an object: %v, %v", ok, existsErr)
	}
	// And no temporary file behind either.
	entries, err := os.ReadDir(filepath.Join(root, "plugins", "code-review"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("leftover files: %v", entries)
	}
}

// Content addressing means a republish of identical bytes is a rewrite of the
// same content, never a replacement of somebody else's.
func TestLocalFSPluginPackagePutTwiceKeepsTheContent(t *testing.T) {
	s := NewLocalFSPluginPackageStorage(t.TempDir())
	ctx := context.Background()
	key, _ := PluginPackageKey("", "code-review", testDigest)

	for range 2 {
		if err := s.Put(ctx, key, strings.NewReader("package bytes")); err != nil {
			t.Fatal(err)
		}
	}
	rc, _, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "package bytes" {
		t.Errorf("read %q", got)
	}
}

func TestLocalFSPluginPackageRejectsAnUnsafeKey(t *testing.T) {
	s := NewLocalFSPluginPackageStorage(t.TempDir())
	ctx := context.Background()
	for _, key := range []string{"", "/absolute", "../escape.tar.gz", "plugins/../../escape"} {
		if err := s.Put(ctx, key, strings.NewReader("x")); err == nil {
			t.Errorf("Put(%q) was accepted", key)
		}
		if _, _, err := s.Open(ctx, key); err == nil {
			t.Errorf("Open(%q) was accepted", key)
		}
		if _, err := s.Exists(ctx, key); err == nil {
			t.Errorf("Exists(%q) was accepted", key)
		}
	}
}

func TestS3PluginPackageStorage(t *testing.T) {
	client := &fakeS3{objects: map[string][]byte{}}
	s := NewS3PluginPackageStorage(client, "bucket")
	ctx := context.Background()
	key, _ := PluginPackageKey("bm", "code-review", testDigest)

	if ok, err := s.Exists(ctx, key); err != nil || ok {
		t.Errorf("Exists before Put = %v, %v", ok, err)
	}
	if err := s.Put(ctx, key, strings.NewReader("package bytes")); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Exists(ctx, key); err != nil || !ok {
		t.Errorf("Exists after Put = %v, %v", ok, err)
	}

	rc, size, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if size != int64(len("package bytes")) {
		t.Errorf("size = %d", size)
	}
	got, _ := io.ReadAll(rc)
	if string(got) != "package bytes" {
		t.Errorf("read %q", got)
	}

	if _, _, err := s.Open(ctx, "bm/plugins/absent/sha256-x.tar.gz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing object: err = %v, want ErrNotFound", err)
	}
	if err := s.Put(ctx, "../escape", strings.NewReader("x")); err == nil {
		t.Error("an unsafe key was accepted")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

type fakeS3 struct{ objects map[string][]byte }

func (f *fakeS3) PutObject(_ context.Context, _, key string, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	f.objects[key] = data
	return nil
}

func (f *fakeS3) GetObject(_ context.Context, _, key string) ([]byte, error) {
	data, ok := f.objects[key]
	if !ok {
		return nil, ErrNotFound
	}
	return data, nil
}

func (f *fakeS3) ListObjectKeys(_ context.Context, _, prefix string) ([]string, error) {
	var keys []string
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (f *fakeS3) GetObjectStream(_ context.Context, _, key string) (io.ReadCloser, int64, error) {
	data, ok := f.objects[key]
	if !ok {
		return nil, 0, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (f *fakeS3) ObjectExists(_ context.Context, _, key string) (bool, error) {
	_, ok := f.objects[key]
	return ok, nil
}

func (f *fakeS3) DeleteObject(_ context.Context, _, key string) error {
	delete(f.objects, key)
	return nil
}
