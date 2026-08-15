// Release-archive verification: validate a GoReleaser archive's checksum and
// contents, and smoke-test the binary for the host platform.
//
// This ran as scripts/verify-release-archive.go until it moved here, so that
// every binary entry point in the repository lives under cmd/ and a maintainer
// can verify a release the same way CI does: `./make release verify`.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func cmdVerifyArchive(args []string) error {
	fs := flag.NewFlagSet("release verify", flag.ContinueOnError)
	dist := fs.String("dist", "dist", "directory containing GoReleaser archives")
	all := fs.Bool("all", false, "validate every supported archive without executing binaries")
	goos := fs.String("goos", runtime.GOOS, "target operating system")
	goarch := fs.String("goarch", runtime.GOARCH, "target architecture")
	skipExec := fs.Bool("skip-exec", false, "validate contents without executing the CLI")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *all {
		targets := [][2]string{
			{"linux", "amd64"},
			{"linux", "arm64"},
			{"darwin", "amd64"},
			{"darwin", "arm64"},
			{"windows", "amd64"},
		}
		for _, target := range targets {
			if err := verify(*dist, target[0], target[1], false); err != nil {
				return fmt.Errorf("release archive verification failed: %w", err)
			}
		}
		return nil
	}
	if err := verify(*dist, *goos, *goarch, !*skipExec); err != nil {
		return fmt.Errorf("release archive verification failed: %w", err)
	}
	return nil
}

func verify(dist, goos, goarch string, execute bool) error {
	archivePath, err := findArchive(dist, goos, goarch)
	if err != nil {
		return err
	}
	if err := verifyChecksum(filepath.Join(dist, "checksums.txt"), archivePath); err != nil {
		return err
	}

	target, err := os.MkdirTemp("", "buildmax-release-")
	if err != nil {
		return fmt.Errorf("create extraction directory: %w", err)
	}
	defer os.RemoveAll(target)

	switch {
	case strings.HasSuffix(archivePath, ".tar.gz"):
		err = extractTarGz(archivePath, target)
	case strings.HasSuffix(archivePath, ".zip"):
		err = extractZip(archivePath, target)
	default:
		err = fmt.Errorf("unsupported archive format: %s", archivePath)
	}
	if err != nil {
		return err
	}

	// Not the package-level exe(), which answers for the host: the target
	// platform here is a parameter, so a Linux runner can check the Windows zip.
	exeSuffix := ""
	if goos == "windows" {
		exeSuffix = ".exe"
	}
	required := []string{
		"LICENSE",
		"NOTICE-THIRD-PARTY",
		"README.md",
		"SECURITY.md",
		"CHANGELOG.md",
		filepath.Join("config-examples", "settings.example.yaml"),
		"buildmax" + exeSuffix,
		"buildmax-server" + exeSuffix,
		"buildmax-worker" + exeSuffix,
	}
	for _, name := range required {
		if info, statErr := os.Stat(filepath.Join(target, name)); statErr != nil || info.IsDir() {
			return fmt.Errorf("required release file %q is missing", name)
		}
	}

	version := "contents and checksum"
	if execute {
		if goos != runtime.GOOS || goarch != runtime.GOARCH {
			return fmt.Errorf("cannot execute %s/%s archive on %s/%s", goos, goarch, runtime.GOOS, runtime.GOARCH)
		}
		cli := filepath.Join(target, "buildmax"+exeSuffix)
		if goos != "windows" {
			if err := os.Chmod(cli, 0o755); err != nil {
				return fmt.Errorf("make CLI executable: %w", err)
			}
		}
		output, err := exec.Command(cli, "version").CombinedOutput()
		if err != nil {
			return fmt.Errorf("run buildmax version: %w\n%s", err, output)
		}
		version = strings.TrimSpace(string(output))
		if !strings.HasPrefix(version, "buildmax version ") {
			return fmt.Errorf("unexpected version output: %q", version)
		}
	}

	fmt.Printf("Verified %s (%s/%s): %s\n", filepath.Base(archivePath), goos, goarch, version)
	return nil
}

func findArchive(dist, goos, goarch string) (string, error) {
	patterns := []string{
		filepath.Join(dist, fmt.Sprintf("buildmax_*_%s_%s.tar.gz", goos, goarch)),
		filepath.Join(dist, fmt.Sprintf("buildmax_*_%s_%s.zip", goos, goarch)),
	}
	var matches []string
	for _, pattern := range patterns {
		found, err := filepath.Glob(pattern)
		if err != nil {
			return "", fmt.Errorf("match release archive: %w", err)
		}
		matches = append(matches, found...)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("expected one %s/%s archive in %s, found %d", goos, goarch, dist, len(matches))
	}
	return matches[0], nil
}

func verifyChecksum(checksumsPath, archivePath string) error {
	body, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
	want := ""
	name := filepath.Base(archivePath)
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("checksum for %s is missing", name)
	}
	if _, err := hex.DecodeString(want); err != nil || len(want) != sha256.Size*2 {
		return fmt.Errorf("checksum for %s is not SHA-256", name)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive for checksum: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash archive: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s", name)
	}
	return nil
}

func extractTarGz(archivePath, target string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open tar archive: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar archive: %w", err)
		}
		dst, err := archiveDestination(target, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("create archive directory: %w", err)
			}
		case tar.TypeReg:
			if err := writeArchiveFile(dst, tr, os.FileMode(h.Mode)); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported tar entry %q", h.Name)
		}
	}
}

func extractZip(archivePath, target string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		dst, err := archiveDestination(target, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("create archive directory: %w", err)
			}
			continue
		}
		r, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}
		err = writeArchiveFile(dst, r, f.Mode())
		closeErr := r.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return fmt.Errorf("close zip entry %q: %w", f.Name, closeErr)
		}
	}
	return nil
}

// rootedOrDriveRelative reports whether an archive entry names a location that
// is not relative to the extraction directory. filepath.IsAbs cannot answer
// this on its own: on Windows `\name` is rooted but not absolute and `C:name`
// is relative to a drive's working directory, while on Unix both are ordinary
// filenames. The check is spelled out against the raw entry name so it behaves
// identically on every platform — a release archive never contains either
// shape, and a Windows-only rule is one nobody exercises until CI turns red.
func rootedOrDriveRelative(name string) bool {
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return true
	}
	return len(name) >= 2 && name[1] == ':' &&
		(name[0] >= 'a' && name[0] <= 'z' || name[0] >= 'A' && name[0] <= 'Z')
}

func archiveDestination(target, name string) (string, error) {
	if rootedOrDriveRelative(name) {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	dst := filepath.Join(target, clean)
	rel, err := filepath.Rel(target, dst)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return dst, nil
}

func writeArchiveFile(dst string, src io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create archive parent: %w", err)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("create archive file: %w", err)
	}
	_, copyErr := io.Copy(f, src)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("extract archive file: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close archive file: %w", closeErr)
	}
	return nil
}
