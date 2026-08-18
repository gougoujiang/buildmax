package main

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveDestination(t *testing.T) {
	target := t.TempDir()
	valid, err := archiveDestination(target, "config-examples/settings.example.yaml")
	if err != nil {
		t.Fatalf("valid path rejected: %v", err)
	}
	if !strings.HasPrefix(valid, target+string(filepath.Separator)) {
		t.Fatalf("destination %q escaped %q", valid, target)
	}

	// Spelled out rather than built from filepath.Separator, so every platform
	// checks every shape. `\absolute` and `C:absolute` are the Windows forms
	// filepath.IsAbs does not catch, and a Windows-only assertion is one no one
	// runs until CI turns red.
	unsafe := []string{
		"../escape",
		filepath.Join("..", "escape"),
		"config-examples/../../escape",
		"/absolute",
		`\absolute`,
		"C:/absolute",
		`C:\absolute`,
		"C:absolute",
	}
	for _, name := range unsafe {
		if _, err := archiveDestination(target, name); err == nil {
			t.Errorf("unsafe path %q was accepted", name)
		}
	}
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "buildmax_test_linux_amd64.tar.gz")
	content := []byte("release archive")
	if err := os.WriteFile(archivePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	checksumsPath := filepath.Join(dir, "checksums.txt")
	line := fmt.Sprintf("%x  %s\n", sum, filepath.Base(archivePath))
	if err := os.WriteFile(checksumsPath, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum(checksumsPath, archivePath); err != nil {
		t.Fatalf("valid checksum rejected: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum(checksumsPath, archivePath); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
}

func TestExtractZipRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "unsafe.zip")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	entry, err := zw.Create("../escape")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("unsafe")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(dir, "target")
	if err := extractZip(archivePath, target); err == nil {
		t.Fatal("path traversal archive was accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "escape")); !os.IsNotExist(err) {
		t.Fatal("path traversal wrote outside the extraction directory")
	}
}
