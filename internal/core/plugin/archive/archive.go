// Package archive packs and extracts a plugin package.
//
// A published release is bytes somebody else produced, so extraction is the
// place this feature can be attacked. Every guard here exists because a tar
// reader that trusts its input will happily write outside the directory it was
// given, follow a link into somebody's home, or expand a kilobyte into a disk.
//
// It depends only on the standard library so both sides can run it: the client
// packs and installs, and internal/server validates the same bytes without
// reaching into internal/config.
package archive

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// DigestPrefix labels the one hash this format uses. Recording the algorithm
// beside the value means a later change cannot be mistaken for a mismatch.
const DigestPrefix = "sha256:"

// GitDir is excluded when packing. A checkout's history is not plugin content,
// it is usually larger than everything that is, and it carries branches and
// remotes the author never meant to publish.
const GitDir = ".git"

// Violations extraction refuses. Each is a way an archive can reach outside
// what it was given.
var (
	ErrTraversal    = errors.New("archive entry escapes the destination")
	ErrLink         = errors.New("archive contains a link")
	ErrIrregular    = errors.New("archive contains a file that is not a regular file or directory")
	ErrDuplicate    = errors.New("archive contains the same path twice")
	ErrTooManyFiles = errors.New("archive holds more files than the limit allows")
	ErrTooLarge     = errors.New("archive is larger than the limit allows")
	ErrPathTooLong  = errors.New("archive entry path is longer than the limit allows")
	ErrDigest       = errors.New("digest does not match")
)

// Limits bound what an archive may cost to accept. Zero fields take the
// default, so a caller cannot accidentally disable a guard by leaving one out.
type Limits struct {
	MaxFiles           int
	MaxFileBytes       int64
	MaxTotalBytes      int64
	MaxCompressedBytes int64
	MaxPathLength      int
}

// DefaultLimits is generous for a directory of instructions and scripts, and
// far below anything that could exhaust a disk.
var DefaultLimits = Limits{
	MaxFiles:           2000,
	MaxFileBytes:       16 << 20,
	MaxTotalBytes:      64 << 20,
	MaxCompressedBytes: 32 << 20,
	MaxPathLength:      512,
}

// ResolveLimits fills in the zero fields of lim, so a caller outside this
// package can see the same bounds extraction will apply.
func ResolveLimits(lim Limits) Limits { return lim.withDefaults() }

func (l Limits) withDefaults() Limits {
	d := DefaultLimits
	if l.MaxFiles > 0 {
		d.MaxFiles = l.MaxFiles
	}
	if l.MaxFileBytes > 0 {
		d.MaxFileBytes = l.MaxFileBytes
	}
	if l.MaxTotalBytes > 0 {
		d.MaxTotalBytes = l.MaxTotalBytes
	}
	if l.MaxCompressedBytes > 0 {
		d.MaxCompressedBytes = l.MaxCompressedBytes
	}
	if l.MaxPathLength > 0 {
		d.MaxPathLength = l.MaxPathLength
	}
	return d
}

// Summary describes what was packed or extracted.
type Summary struct {
	Files int
	Bytes int64
	// Digest is set by Pack, over exactly the bytes it wrote.
	Digest string
}

// Pack writes root as a gzipped tar and returns the digest of what it wrote.
//
// The output is deterministic: entries are walked in lexical order, timestamps
// and ownership are zeroed, and modes are normalised. Packing one tree twice
// therefore produces one digest, which is what lets a publisher and a consumer
// compare notes about the same release.
func Pack(w io.Writer, root fs.FS, lim Limits) (Summary, error) {
	lim = lim.withDefaults()
	hash := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(w, hash))
	tw := tar.NewWriter(gz)

	var sum Summary
	err := fs.WalkDir(root, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if d.IsDir() && path.Base(name) == GitDir {
			return fs.SkipDir
		}
		if len(name) > lim.MaxPathLength {
			return fmt.Errorf("%w: %s", ErrPathTooLong, name)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return tw.WriteHeader(&tar.Header{
				Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o755, Format: tar.FormatPAX,
			})
		case !info.Mode().IsRegular():
			// A symlink in a package would be resolved against whatever machine
			// unpacked it, so it cannot mean the same thing twice.
			return fmt.Errorf("%w: %s", ErrIrregular, name)
		}

		sum.Files++
		if sum.Files > lim.MaxFiles {
			return ErrTooManyFiles
		}
		if info.Size() > lim.MaxFileBytes {
			return fmt.Errorf("%w: %s", ErrTooLarge, name)
		}
		sum.Bytes += info.Size()
		if sum.Bytes > lim.MaxTotalBytes {
			return ErrTooLarge
		}

		if err := tw.WriteHeader(&tar.Header{
			Name: name, Typeflag: tar.TypeReg, Mode: fileMode(info.Mode()),
			Size: info.Size(), Format: tar.FormatPAX,
		}); err != nil {
			return err
		}
		f, err := root.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		written, err := io.Copy(tw, f)
		if err != nil {
			return err
		}
		if written != info.Size() {
			return fmt.Errorf("%s changed while being packed", name)
		}
		return nil
	})
	if err != nil {
		return Summary{}, err
	}
	if err := tw.Close(); err != nil {
		return Summary{}, err
	}
	if err := gz.Close(); err != nil {
		return Summary{}, err
	}
	sum.Digest = DigestPrefix + hex.EncodeToString(hash.Sum(nil))
	return sum, nil
}

// fileMode keeps only the executable bit, so setuid, setgid, and sticky cannot
// travel in a package and a group-writable file cannot arrive that way.
func fileMode(m fs.FileMode) int64 {
	if m&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

// Digest reads r fully and returns its labelled hash.
func Digest(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return DigestPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyDigest reports whether r hashes to want.
func VerifyDigest(r io.Reader, want string) error {
	got, err := Digest(r)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("%w: have %s, want %s", ErrDigest, got, want)
	}
	return nil
}

// Extract writes an archive into destDir, which must already exist.
//
// Nothing in the archive decides where a byte lands: every path is cleaned and
// checked against the destination before a file is opened, links are refused
// outright rather than resolved, and modes come from the extractor rather than
// from the header.
func Extract(r io.Reader, destDir string, lim Limits) (Summary, error) {
	lim = lim.withDefaults()
	root, err := filepath.Abs(destDir)
	if err != nil {
		return Summary{}, err
	}

	// Bounding the compressed stream first stops an endless one before gzip
	// ever has to decide what it means.
	bounded := &limitedReader{r: r, remaining: lim.MaxCompressedBytes, err: ErrTooLarge}
	gz, err := gzip.NewReader(bounded)
	if err != nil {
		return Summary{}, fmt.Errorf("read archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	var sum Summary

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Summary{}, fmt.Errorf("read archive: %w", err)
		}

		rel, err := safeRelPath(header.Name, lim.MaxPathLength)
		if err != nil {
			return Summary{}, err
		}
		if seen[rel] {
			// One path with two entries reads differently depending on whether
			// a reader keeps the first or the last, which is exactly how a
			// validator and an extractor get told different stories.
			return Summary{}, fmt.Errorf("%w: %s", ErrDuplicate, rel)
		}
		seen[rel] = true

		target := filepath.Join(root, filepath.FromSlash(rel))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return Summary{}, err
			}
		case tar.TypeReg:
			sum.Files++
			if sum.Files > lim.MaxFiles {
				return Summary{}, ErrTooManyFiles
			}
			written, err := writeFile(tr, target, fs.FileMode(fileMode(fs.FileMode(header.Mode))), lim, sum.Bytes)
			if err != nil {
				return Summary{}, err
			}
			sum.Bytes += written
		case tar.TypeSymlink, tar.TypeLink:
			return Summary{}, fmt.Errorf("%w: %s", ErrLink, rel)
		default:
			return Summary{}, fmt.Errorf("%w: %s", ErrIrregular, rel)
		}
	}
	return sum, nil
}

// writeFile copies one entry, stopping on the bytes it actually writes.
//
// The declared size is never consulted. Go's tar reader already bounds an entry
// to what its header claimed, so this is the second line rather than the first
// — but a limit that trusted the header would be checking the attacker's
// arithmetic instead of its own.
func writeFile(r io.Reader, target string, mode fs.FileMode, lim Limits, written int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	remaining := lim.MaxFileBytes
	if left := lim.MaxTotalBytes - written; left < remaining {
		remaining = left
	}
	// Reading one byte past the allowance is what separates "exactly at the
	// limit" from "over it".
	n, err := io.Copy(f, io.LimitReader(r, remaining+1))
	if err != nil {
		return 0, err
	}
	if n > remaining {
		return 0, fmt.Errorf("%w: %s", ErrTooLarge, filepath.Base(target))
	}
	return n, nil
}

// safeRelPath reduces an archive path to one that can only land inside the
// destination, or refuses it.
func safeRelPath(name string, maxLen int) (string, error) {
	if len(name) > maxLen {
		return "", fmt.Errorf("%w: %s", ErrPathTooLong, name)
	}
	// A backslash is a separator on Windows and an ordinary character
	// elsewhere, so the same archive would mean two things.
	if strings.ContainsAny(name, `\`) || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: %s", ErrTraversal, name)
	}
	if path.IsAbs(name) || filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("%w: %s", ErrTraversal, name)
	}
	// A Windows drive or UNC prefix is absolute there and relative here.
	if len(name) > 1 && name[1] == ':' {
		return "", fmt.Errorf("%w: %s", ErrTraversal, name)
	}
	clean := path.Clean(strings.TrimSuffix(name, "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %s", ErrTraversal, name)
	}
	return clean, nil
}

// limitedReader fails once the stream passes its allowance, instead of
// reporting a clean end that would look like a complete archive.
type limitedReader struct {
	r         io.Reader
	remaining int64
	err       error
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.remaining <= 0 {
		return 0, l.err
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.r.Read(p)
	l.remaining -= int64(n)
	return n, err
}
