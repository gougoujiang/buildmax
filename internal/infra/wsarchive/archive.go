// Package wsarchive encodes and decodes a Task workspace checkpoint's payload:
// one Zstandard-compressed tar archive, format tar.zst.v1. It is pure
// infrastructure — encoding, canonicalization, and adversarial validation — and
// owns no domain state. See docs/design/task-workspace-checkpoints.md §7.
//
// Capture and extraction apply the same validation independently: an archive is
// never trusted merely because BuildMax produced it, so object-store corruption
// cannot become a filesystem escape.
package wsarchive

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// Limits bound a capture or extraction so a decompression bomb or an unbounded
// tree cannot exhaust the worker. A zero field means that dimension is
// unbounded, which a caller should avoid for untrusted extraction.
type Limits struct {
	MaxUncompressedBytes int64 // sum of regular-file bytes
	MaxEntries           int64 // regular files, directories, and symlinks
	MaxPathDepth         int   // slash-separated components in any entry path
}

// Result reports what a capture wrote or an extraction materialized.
type Result struct {
	UncompressedBytes int64
	EntryCount        int64
}

var (
	// ErrUnsafeEntry is any entry the canonical rules forbid: an absolute or
	// escaping path, a hard link, a device node, a socket, a FIFO, or a symlink
	// that leaves the root. See §7.2.
	ErrUnsafeEntry = errors.New("wsarchive: unsafe archive entry")
	// ErrLimitExceeded is a capture or extraction that ran past a Limit.
	ErrLimitExceeded = errors.New("wsarchive: archive limit exceeded")
)

// Create walks srcDir in lexical order and writes a tar.zst.v1 archive to w,
// returning the counters the checkpoint records. Every header path is relative
// to srcDir. It preserves regular-file bytes, directories, executable and
// ordinary permission bits, modification time, and safe relative symlinks;
// it normalizes owner/group to zero and strips setuid, setgid, and sticky bits.
// It fails on any entry the canonical rules forbid (§7.2).
//
// Create does not compute the payload digest: the caller tees w through a hash
// so the digest covers the exact stored bytes, per the commit protocol (§8).
func Create(w io.Writer, srcDir string, limits Limits) (Result, error) {
	root, err := filepath.Abs(srcDir)
	if err != nil {
		return Result{}, err
	}
	zw, err := zstd.NewWriter(w)
	if err != nil {
		return Result{}, err
	}
	tw := tar.NewWriter(zw)

	var res Result
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil // the root itself is not an entry
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if err := validateName(name, limits.MaxPathDepth); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}

		hdr, size, err := headerFor(name, p, info)
		if err != nil {
			return err
		}
		res.EntryCount++
		res.UncompressedBytes += size
		if err := checkLimits(res, limits); err != nil {
			return err
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeReg {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			n, copyErr := io.Copy(tw, f)
			_ = f.Close()
			if copyErr != nil {
				return copyErr
			}
			if n != size {
				return fmt.Errorf("wsarchive: %s changed size during capture", name)
			}
		}
		return nil
	})
	if walkErr != nil {
		_ = tw.Close()
		_ = zw.Close()
		return Result{}, walkErr
	}
	if err := tw.Close(); err != nil {
		_ = zw.Close()
		return Result{}, err
	}
	if err := zw.Close(); err != nil {
		return Result{}, err
	}
	return res, nil
}

// headerFor builds a normalized tar header for one filesystem entry and returns
// the regular-file byte count (zero for a directory or symlink). It rejects any
// unsupported type and any unsafe symlink.
func headerFor(name, fsPath string, info os.FileInfo) (*tar.Header, int64, error) {
	mode := info.Mode()
	switch {
	case mode.IsRegular():
		if isHardLinked(info) {
			return nil, 0, fmt.Errorf("%w: hard link %s", ErrUnsafeEntry, name)
		}
		return &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     name,
			Mode:     int64(mode.Perm()), // Perm() already drops setuid/setgid/sticky
			Size:     info.Size(),
			ModTime:  info.ModTime().UTC(),
		}, info.Size(), nil
	case mode.IsDir():
		return &tar.Header{
			Typeflag: tar.TypeDir,
			Name:     name + "/",
			Mode:     int64(mode.Perm()),
			ModTime:  info.ModTime().UTC(),
		}, 0, nil
	case mode&os.ModeSymlink != 0:
		target, err := os.Readlink(fsPath)
		if err != nil {
			return nil, 0, err
		}
		if err := validateSymlink(name, target); err != nil {
			return nil, 0, err
		}
		return &tar.Header{
			Typeflag: tar.TypeSymlink,
			Name:     name,
			Linkname: filepath.ToSlash(target),
			Mode:     int64(mode.Perm()),
			ModTime:  info.ModTime().UTC(),
		}, 0, nil
	default:
		// Device nodes, sockets, FIFOs, and anything else are not a workspace.
		return nil, 0, fmt.Errorf("%w: unsupported file type for %s", ErrUnsafeEntry, name)
	}
}

// Extract reads a tar.zst.v1 archive from r and materializes it into destDir,
// applying the same validation independently and enforcing limits. destDir must
// already exist and be empty; the caller extracts into a temporary directory
// and renames it into place so the Agent never sees a half-extracted workspace.
func Extract(r io.Reader, destDir string, limits Limits) (Result, error) {
	root, err := filepath.Abs(destDir)
	if err != nil {
		return Result{}, err
	}
	zr, err := zstd.NewReader(r)
	if err != nil {
		return Result{}, err
	}
	defer zr.Close()
	tr := tar.NewReader(zr)

	seen := map[string]bool{}
	var res Result
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Result{}, err
		}
		name := path.Clean(strings.TrimSuffix(hdr.Name, "/"))
		if err := validateName(name, limits.MaxPathDepth); err != nil {
			return Result{}, err
		}
		if seen[name] {
			return Result{}, fmt.Errorf("%w: duplicate path %s", ErrUnsafeEntry, name)
		}
		seen[name] = true

		target := filepath.Join(root, filepath.FromSlash(name))
		res.EntryCount++
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode).Perm()); err != nil {
				return Result{}, err
			}
		case tar.TypeReg:
			res.UncompressedBytes += hdr.Size
			if err := checkLimits(res, limits); err != nil {
				return Result{}, err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return Result{}, err
			}
			if err := writeFile(target, tr, os.FileMode(hdr.Mode).Perm(), hdr.Size, limits); err != nil {
				return Result{}, err
			}
			_ = os.Chtimes(target, hdr.ModTime, hdr.ModTime)
		case tar.TypeSymlink:
			if err := validateSymlink(name, hdr.Linkname); err != nil {
				return Result{}, err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return Result{}, err
			}
			if err := os.Symlink(filepath.FromSlash(hdr.Linkname), target); err != nil {
				return Result{}, err
			}
		default:
			return Result{}, fmt.Errorf("%w: unsupported tar type %d for %s", ErrUnsafeEntry, hdr.Typeflag, name)
		}
		if err := checkLimits(res, limits); err != nil {
			return Result{}, err
		}
	}
	return res, nil
}

// writeFile copies exactly size bytes, refusing an entry whose stream is longer
// than its header claims — the decompression-bomb defense on the read side.
func writeFile(target string, r io.Reader, perm os.FileMode, size int64, limits Limits) error {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	// One byte over the declared size is a lying header.
	n, err := io.Copy(f, io.LimitReader(r, size+1))
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if n != size {
		return fmt.Errorf("%w: entry stream did not match its declared size", ErrUnsafeEntry)
	}
	return nil
}

// validateName rejects absolute paths, empty paths, `..` traversal, and paths
// deeper than the limit. The result of path.Clean starting with ".." or being
// absolute is the escape test.
func validateName(name string, maxDepth int) error {
	if name == "" || name == "." {
		return fmt.Errorf("%w: empty path", ErrUnsafeEntry)
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return fmt.Errorf("%w: absolute path %s", ErrUnsafeEntry, name)
	}
	cleaned := path.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("%w: path escapes the root: %s", ErrUnsafeEntry, name)
	}
	if maxDepth > 0 && strings.Count(cleaned, "/")+1 > maxDepth {
		return fmt.Errorf("%w: path deeper than %d: %s", ErrLimitExceeded, maxDepth, name)
	}
	return nil
}

// validateSymlink rejects an absolute link target and a relative one that,
// resolved against the link's own directory, escapes the archive root.
func validateSymlink(name, target string) error {
	if target == "" {
		return fmt.Errorf("%w: empty symlink target for %s", ErrUnsafeEntry, name)
	}
	if filepath.IsAbs(target) || strings.HasPrefix(target, "/") {
		return fmt.Errorf("%w: absolute symlink %s -> %s", ErrUnsafeEntry, name, target)
	}
	resolved := path.Clean(path.Join(path.Dir(filepath.ToSlash(name)), filepath.ToSlash(target)))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("%w: symlink escapes the root: %s -> %s", ErrUnsafeEntry, name, target)
	}
	return nil
}

func checkLimits(res Result, limits Limits) error {
	if limits.MaxEntries > 0 && res.EntryCount > limits.MaxEntries {
		return fmt.Errorf("%w: more than %d entries", ErrLimitExceeded, limits.MaxEntries)
	}
	if limits.MaxUncompressedBytes > 0 && res.UncompressedBytes > limits.MaxUncompressedBytes {
		return fmt.Errorf("%w: more than %d uncompressed bytes", ErrLimitExceeded, limits.MaxUncompressedBytes)
	}
	return nil
}

// PayloadFormat is the archive format string recorded on a checkpoint row.
const PayloadFormat = "tar.zst.v1"
