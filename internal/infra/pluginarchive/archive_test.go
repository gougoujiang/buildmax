package pluginarchive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	inspect "github.com/gougoujiang/buildmax/internal/service/plugininspect"
)

func file(body string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(body)} }

func packTo(t *testing.T, root fs.FS) ([]byte, Summary) {
	t.Helper()
	var buf bytes.Buffer
	sum, err := Pack(&buf, root, Limits{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	return buf.Bytes(), sum
}

// entry is one handwritten archive record, for the shapes Pack refuses to
// produce and Extract has to refuse to accept.
type entry struct {
	name     string
	typeflag byte
	mode     int64
	body     string
	linkname string
	// size overrides the declared length, for an entry that ends early.
	size int64
}

func buildArchive(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typeflag := e.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		size := int64(len(e.body))
		if e.size != 0 {
			size = e.size
		}
		if err := tw.WriteHeader(&tar.Header{
			Name: e.name, Typeflag: typeflag, Mode: mode, Size: size, Linkname: e.linkname,
		}); err != nil {
			t.Fatal(err)
		}
		if typeflag == tar.TypeReg {
			if _, err := io.WriteString(tw, e.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A header that lies about its size leaves the writer inconsistent, which
	// is the point of the test rather than a problem with it.
	_ = tw.Close()
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPackExtractRoundTrip(t *testing.T) {
	src := fstest.MapFS{
		"plugin.yaml":            file("name: code-review\n"),
		"skills/review/SKILL.md": file("# review\n"),
		"hooks/format.sh":        &fstest.MapFile{Data: []byte("#!/bin/sh\n"), Mode: 0o755},
	}
	data, sum := packTo(t, src)
	if sum.Files != 3 {
		t.Errorf("packed %d files, want 3", sum.Files)
	}
	if !strings.HasPrefix(sum.Digest, DigestPrefix) || len(sum.Digest) != len(DigestPrefix)+64 {
		t.Errorf("Digest = %q", sum.Digest)
	}

	dest := t.TempDir()
	if _, err := Extract(bytes.NewReader(data), dest, Limits{}); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	for name, want := range map[string]string{
		"plugin.yaml":            "name: code-review\n",
		"skills/review/SKILL.md": "# review\n",
		"hooks/format.sh":        "#!/bin/sh\n",
	} {
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	assertExtractedModes(t, dest)
}

// assertExtractedModes checks that the executable bit survives extraction and
// nothing else about the mode does.
//
// Windows has no Unix permission bits — every extracted file reads back
// 0666 — so there is nothing to assert there. What the archive carries is
// still checked: TestPackNormalisesModes reads the header rather than the
// filesystem, and runs everywhere.
func assertExtractedModes(t *testing.T, dest string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not carry Unix permission bits")
	}
	info, err := os.Stat(filepath.Join(dest, "hooks", "format.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("script mode = %v, want 0755", info.Mode().Perm())
	}
	plain, err := os.Stat(filepath.Join(dest, "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if plain.Mode().Perm() != 0o644 {
		t.Errorf("file mode = %v, want 0644", plain.Mode().Perm())
	}
}

// Only the executable bit travels, so setuid, setgid, and sticky cannot arrive
// in a package and a group-writable file cannot either. This reads the archive
// rather than the filesystem, so it holds on every platform.
func TestPackNormalisesModes(t *testing.T) {
	data, _ := packTo(t, fstest.MapFS{
		"plain.md":     file("x"),
		"script.sh":    &fstest.MapFile{Data: []byte("#!/bin/sh\n"), Mode: 0o755},
		"dangerous.sh": &fstest.MapFile{Data: []byte("#!/bin/sh\n"), Mode: 0o4777 | fs.ModeSetuid},
	})
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()

	modes := map[string]int64{}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		modes[header.Name] = header.Mode
	}
	want := map[string]int64{"plain.md": 0o644, "script.sh": 0o755, "dangerous.sh": 0o755}
	for name, mode := range want {
		if modes[name] != mode {
			t.Errorf("%s mode = %#o, want %#o", name, modes[name], mode)
		}
	}
}

// A publisher and a consumer have to be able to compare notes about the same
// release, which only works if packing one tree twice gives one digest.
func TestPackIsDeterministic(t *testing.T) {
	src := fstest.MapFS{
		"plugin.yaml":            file("name: code-review\n"),
		"skills/review/SKILL.md": file("# review\n"),
	}
	_, first := packTo(t, src)
	_, second := packTo(t, src)
	if first.Digest != second.Digest {
		t.Errorf("digests differ across packs: %s vs %s", first.Digest, second.Digest)
	}

	changed := fstest.MapFS{
		"plugin.yaml":            file("name: code-review\n"),
		"skills/review/SKILL.md": file("# review, edited\n"),
	}
	_, third := packTo(t, changed)
	if third.Digest == first.Digest {
		t.Error("a changed tree produced the same digest")
	}
}

// History is not plugin content, and it carries branches and remotes the author
// never meant to publish.
func TestPackExcludesGitDirectory(t *testing.T) {
	src := fstest.MapFS{
		"plugin.yaml":     file("name: cloned\n"),
		".git/config":     file("[remote \"origin\"]\n"),
		".git/objects/ab": file("binary"),
		".gitignore":      file("*.log\n"),
	}
	data, sum := packTo(t, src)
	if sum.Files != 2 {
		t.Errorf("packed %d files, want plugin.yaml and .gitignore", sum.Files)
	}
	dest := t.TempDir()
	if _, err := Extract(bytes.NewReader(data), dest, Limits{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Error(".git reached the package")
	}
	if _, err := os.Stat(filepath.Join(dest, ".gitignore")); err != nil {
		t.Error("an ordinary dot file should still be packed")
	}
}

// Nothing in the archive decides where a byte lands.
func TestExtractRefusesTraversal(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"parent", "../escaped.txt"},
		{"deep parent", "a/b/../../../escaped.txt"},
		{"absolute", "/etc/passwd"},
		{"bare dotdot", ".."},
		{"windows drive", `C:/escaped.txt`},
		{"backslash separator", `..\escaped.txt`},
		{"backslash inside", `dir\file.txt`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dest := t.TempDir()
			outside := filepath.Join(dest, "..", "escaped.txt")
			data := buildArchive(t, []entry{{name: tc.path, body: "owned"}})

			if _, err := Extract(bytes.NewReader(data), dest, Limits{}); !errors.Is(err, ErrTraversal) {
				t.Fatalf("err = %v, want ErrTraversal", err)
			}
			// Refused before anything opened a file, not cleaned up after.
			if _, err := os.Stat(outside); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s exists; the entry was written before it was refused", outside)
			}
		})
	}
}

// A link would be resolved against whatever machine unpacked it, so it cannot
// mean the same thing twice — and one of those meanings is somebody's home.
func TestExtractRefusesLinksAndIrregularFiles(t *testing.T) {
	tests := []struct {
		name     string
		typeflag byte
		linkname string
		want     error
	}{
		{"symlink", tar.TypeSymlink, "/etc/passwd", ErrLink},
		{"hardlink", tar.TypeLink, "plugin.yaml", ErrLink},
		{"fifo", tar.TypeFifo, "", ErrIrregular},
		{"char device", tar.TypeChar, "", ErrIrregular},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := buildArchive(t, []entry{{name: "evil", typeflag: tc.typeflag, linkname: tc.linkname}})
			if _, err := Extract(bytes.NewReader(data), t.TempDir(), Limits{}); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// One path with two entries reads differently depending on whether a reader
// keeps the first or the last, which is how a validator and an extractor get
// told different stories.
func TestExtractRefusesDuplicatePaths(t *testing.T) {
	data := buildArchive(t, []entry{
		{name: "plugin.yaml", body: "name: honest\n"},
		{name: "plugin.yaml", body: "name: switched\n"},
	})
	if _, err := Extract(bytes.NewReader(data), t.TempDir(), Limits{}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}

	// The same path spelled differently is still the same path.
	data = buildArchive(t, []entry{
		{name: "a/plugin.yaml", body: "one"},
		{name: "./a/plugin.yaml", body: "two"},
	})
	if _, err := Extract(bytes.NewReader(data), t.TempDir(), Limits{}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate for a re-spelled path", err)
	}
}

func TestExtractEnforcesLimits(t *testing.T) {
	t.Run("file count", func(t *testing.T) {
		var entries []entry
		for i := range 5 {
			entries = append(entries, entry{name: string(rune('a'+i)) + ".md", body: "x"})
		}
		data := buildArchive(t, entries)
		_, err := Extract(bytes.NewReader(data), t.TempDir(), Limits{MaxFiles: 3})
		if !errors.Is(err, ErrTooManyFiles) {
			t.Errorf("err = %v, want ErrTooManyFiles", err)
		}
	})

	t.Run("single file size", func(t *testing.T) {
		data := buildArchive(t, []entry{{name: "big.md", body: strings.Repeat("x", 100)}})
		_, err := Extract(bytes.NewReader(data), t.TempDir(), Limits{MaxFileBytes: 10})
		if !errors.Is(err, ErrTooLarge) {
			t.Errorf("err = %v, want ErrTooLarge", err)
		}
	})

	t.Run("total size", func(t *testing.T) {
		data := buildArchive(t, []entry{
			{name: "a.md", body: strings.Repeat("x", 60)},
			{name: "b.md", body: strings.Repeat("x", 60)},
		})
		_, err := Extract(bytes.NewReader(data), t.TempDir(), Limits{MaxFileBytes: 100, MaxTotalBytes: 100})
		if !errors.Is(err, ErrTooLarge) {
			t.Errorf("err = %v, want ErrTooLarge", err)
		}
	})

	t.Run("compressed size", func(t *testing.T) {
		data := buildArchive(t, []entry{{name: "a.md", body: strings.Repeat("x", 4096)}})
		_, err := Extract(bytes.NewReader(data), t.TempDir(), Limits{MaxCompressedBytes: 16})
		if !errors.Is(err, ErrTooLarge) {
			t.Errorf("err = %v, want ErrTooLarge", err)
		}
	})

	t.Run("path length", func(t *testing.T) {
		data := buildArchive(t, []entry{{name: strings.Repeat("a", 200) + ".md", body: "x"}})
		_, err := Extract(bytes.NewReader(data), t.TempDir(), Limits{MaxPathLength: 64})
		if !errors.Is(err, ErrPathTooLong) {
			t.Errorf("err = %v, want ErrPathTooLong", err)
		}
	})
}

// A decompressor that expands a kilobyte into a disk is stopped by what it
// produces, not by what it claimed it would produce.
func TestExtractStopsAZipBomb(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	const declared = 64 << 20
	if err := tw.WriteHeader(&tar.Header{Name: "bomb.bin", Typeflag: tar.TypeReg, Mode: 0o644, Size: declared}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(tw, io.LimitReader(zeroes{}, declared)); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	_, err := Extract(bytes.NewReader(buf.Bytes()), dest, Limits{MaxFileBytes: 1024, MaxTotalBytes: 1024})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	info, statErr := os.Stat(filepath.Join(dest, "bomb.bin"))
	if statErr == nil && info.Size() > 4096 {
		t.Errorf("wrote %d bytes before stopping", info.Size())
	}
}

// An entry that ends early is refused rather than landing as a short file that
// looks like the real one.
func TestExtractRefusesATruncatedEntry(t *testing.T) {
	data := buildArchive(t, []entry{{name: "short.md", body: "ten bytes.", size: 4096}})
	dest := t.TempDir()
	if _, err := Extract(bytes.NewReader(data), dest, Limits{}); err == nil {
		t.Fatal("a truncated entry should be refused")
	}
}

func TestVerifyDigest(t *testing.T) {
	data, sum := packTo(t, fstest.MapFS{"plugin.yaml": file("name: x\n")})

	if err := VerifyDigest(bytes.NewReader(data), sum.Digest); err != nil {
		t.Errorf("matching bytes should verify: %v", err)
	}
	if err := VerifyDigest(bytes.NewReader(data[:len(data)-1]), sum.Digest); !errors.Is(err, ErrDigest) {
		t.Errorf("truncated bytes: err = %v, want ErrDigest", err)
	}
	modified := append([]byte(nil), data...)
	modified[len(modified)/2] ^= 0xff
	if err := VerifyDigest(bytes.NewReader(modified), sum.Digest); !errors.Is(err, ErrDigest) {
		t.Errorf("modified bytes: err = %v, want ErrDigest", err)
	}
}

func TestExtractRejectsGarbage(t *testing.T) {
	if _, err := Extract(strings.NewReader("not an archive"), t.TempDir(), Limits{}); err == nil {
		t.Error("garbage should not extract")
	}
}

type zeroes struct{}

func (zeroes) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// The two halves have to fit: what comes out of an archive is what inspection
// reads, with no step in between to reconcile them.
func TestExtractedPackageInspects(t *testing.T) {
	src := fstest.MapFS{
		"plugin.yaml":            file("name: code-review\nversion: 1.2.0\n"),
		"skills/review/SKILL.md": file("# review\n"),
		"agents/reviewer.md":     file("---\nname: reviewer\ndescription: Reviews.\ntools: Read\n---\n\nBody.\n"),
	}
	data, _ := packTo(t, src)
	dest := t.TempDir()
	if _, err := Extract(bytes.NewReader(data), dest, Limits{}); err != nil {
		t.Fatal(err)
	}

	pkg, err := inspect.Dir(os.DirFS(dest))
	if err != nil {
		t.Fatal(err)
	}
	if pkg.HasErrors() {
		t.Fatalf("findings: %+v", pkg.Findings)
	}
	if pkg.Manifest.Name != "code-review" || len(pkg.Skills) != 1 || len(pkg.Subagents) != 1 {
		t.Errorf("inspection = %+v", pkg)
	}
}
