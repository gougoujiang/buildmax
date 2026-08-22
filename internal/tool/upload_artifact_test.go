package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type fakePublisher struct {
	got ArtifactUpload
	res PublishedArtifact
	err error
}

func (f *fakePublisher) PublishArtifact(_ context.Context, in ArtifactUpload) (PublishedArtifact, error) {
	f.got = in
	if f.err != nil {
		return PublishedArtifact{}, f.err
	}
	return f.res, nil
}

func newUploadFixture(t *testing.T) (string, *fakePublisher, *UploadArtifact) {
	t.Helper()
	root := t.TempDir()
	pub := &fakePublisher{res: PublishedArtifact{ArtifactID: "ar_1", Filename: "report.md", SizeBytes: 5, URL: "https://bm.example/api/artifacts/ar_1"}}
	return root, pub, NewUploadArtifact(root, pub)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUploadArtifactPublishesAndReportsTheReference(t *testing.T) {
	root, pub, tool := newUploadFixture(t)
	writeFile(t, filepath.Join(root, "report.md"), "hello")

	out, err := tool.Execute(context.Background(), map[string]any{"path": "report.md", "title": "Quarterly"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if pub.got.Filename != "report.md" {
		t.Errorf("filename = %q", pub.got.Filename)
	}
	if pub.got.Title != "Quarterly" {
		t.Errorf("title = %q", pub.got.Title)
	}
	// The id is what the model has to carry into its answer, so it has to be in
	// the result the model reads.
	if !strings.Contains(out, "ar_1") {
		t.Errorf("result does not name the artifact: %q", out)
	}
	if !strings.Contains(out, "https://bm.example/api/artifacts/ar_1") {
		t.Errorf("result does not carry the URL: %q", out)
	}
}

// Containment is decided lexically, so a link inside the workspace can still
// name a file outside it — and publishing sends the target's bytes to a team.
func TestUploadArtifactRefusesASymlinkOutOfTheWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs elevation on Windows")
	}
	root, pub, tool := newUploadFixture(t)
	outside := filepath.Join(t.TempDir(), "id_rsa")
	writeFile(t, outside, "PRIVATE KEY")
	if err := os.Symlink(outside, filepath.Join(root, "innocent.txt")); err != nil {
		t.Fatal(err)
	}

	_, err := tool.Execute(context.Background(), map[string]any{"path": "innocent.txt"})
	if err == nil {
		t.Fatal("publishing a link out of the workspace must be refused")
	}
	if pub.got.Path != "" {
		t.Errorf("the publisher was reached anyway with %q", pub.got.Path)
	}
}

func TestUploadArtifactAcceptsASymlinkInsideTheWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs elevation on Windows")
	}
	root, _, tool := newUploadFixture(t)
	writeFile(t, filepath.Join(root, "build", "report.md"), "hello")
	if err := os.Symlink(filepath.Join(root, "build", "report.md"), filepath.Join(root, "latest.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "latest.md"}); err != nil {
		t.Errorf("a link inside the workspace should publish its target: %v", err)
	}
}

func TestUploadArtifactRefusesADirectory(t *testing.T) {
	root, _, tool := newUploadFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "dist"}); err == nil {
		t.Error("a directory is not one file to publish")
	}
}

func TestUploadArtifactRefusesAPathOutsideTheWorkspace(t *testing.T) {
	_, _, tool := newUploadFixture(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, outside, "x")
	if _, err := tool.Execute(context.Background(), map[string]any{"path": outside}); err == nil {
		t.Error("an absolute path outside the workspace must be refused")
	}
}

func TestUploadArtifactRefusesANonRegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes are created differently on Windows")
	}
	root, _, tool := newUploadFixture(t)
	fifo := filepath.Join(root, "pipe")
	if err := makeFIFO(fifo); err != nil {
		t.Skipf("cannot create a fifo here: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "pipe"}); err == nil {
		t.Error("a fifo has no content to publish and reading it would block")
	}
}

// A publisher failure has to reach the model as an error, not as a success it
// then cites.
func TestUploadArtifactReportsAPublisherFailure(t *testing.T) {
	root, pub, tool := newUploadFixture(t)
	writeFile(t, filepath.Join(root, "report.md"), "hello")
	pub.err = errors.New("the file is larger than this deployment accepts")

	out, err := tool.Execute(context.Background(), map[string]any{"path": "report.md"})
	if err == nil {
		t.Fatal("a failed publish must be an error")
	}
	if out != "" {
		t.Errorf("a failed publish must return no result text, got %q", out)
	}
	if !strings.Contains(err.Error(), "larger than this deployment accepts") {
		t.Errorf("the server's reason should survive: %v", err)
	}
}

func TestUploadArtifactRequiresAPath(t *testing.T) {
	_, _, tool := newUploadFixture(t)
	if _, err := tool.Execute(context.Background(), map[string]any{}); err == nil {
		t.Error("path is required")
	}
}
