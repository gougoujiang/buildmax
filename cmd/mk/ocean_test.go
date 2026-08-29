package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOceanConfigUsesPersistentResourceDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{
		"BUILDMAX_OCEAN_STATE_DIR",
		"BUILDMAX_OCEAN_PROJECT",
		"BUILDMAX_OCEAN_VPC",
		"BUILDMAX_OCEAN_BUCKET",
		"BUILDMAX_OCEAN_REGION",
	} {
		t.Setenv(name, "")
	}

	cfg, err := loadOceanConfig()
	if err != nil {
		t.Fatalf("loadOceanConfig: %v", err)
	}
	if cfg.project != "buildmax-beta" || cfg.vpc != "buildmax-beta" || cfg.bucket != "buildmax-beta" || cfg.region != "sgp1" {
		t.Fatalf("defaults = %#v", cfg)
	}
	wantStateSuffix := filepath.Join(".buildmax", "qualification", "ocean")
	if filepath.Base(cfg.stateDir) != "ocean" || !pathHasSuffix(cfg.stateDir, wantStateSuffix) {
		t.Errorf("state dir = %q, want suffix %q", cfg.stateDir, wantStateSuffix)
	}
}

func TestOceanConfigAcceptsExplicitPersistentResources(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BUILDMAX_OCEAN_STATE_DIR", stateDir)
	t.Setenv("BUILDMAX_OCEAN_PROJECT", "project-x")
	t.Setenv("BUILDMAX_OCEAN_VPC", "vpc-x")
	t.Setenv("BUILDMAX_OCEAN_BUCKET", "bucket-x")
	t.Setenv("BUILDMAX_OCEAN_REGION", "nyc3")

	cfg, err := loadOceanConfig()
	if err != nil {
		t.Fatalf("loadOceanConfig: %v", err)
	}
	if cfg.project != "project-x" || cfg.vpc != "vpc-x" || cfg.bucket != "bucket-x" || cfg.region != "nyc3" {
		t.Fatalf("overrides = %#v", cfg)
	}
	if cfg.stateDir != stateDir {
		t.Errorf("state dir = %q, want %q", cfg.stateDir, stateDir)
	}
}

func TestOceanStateCannotLiveInRepository(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BUILDMAX_OCEAN_STATE_DIR", filepath.Join(root, ".artifacts", "ocean-state"))
	if _, err := loadOceanConfig(); err == nil {
		t.Fatal("loadOceanConfig accepted state inside the repository")
	}
}

func TestOceanFilesAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	cfg := oceanConfig{stateDir: t.TempDir()}
	for _, path := range []string{oceanStatePath(cfg), oceanKubeconfigPath(cfg), oceanPlanPath(cfg, false)} {
		if err := os.WriteFile(path, []byte("secret"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := protectOceanFiles(cfg); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{oceanStatePath(cfg), oceanKubeconfigPath(cfg), oceanPlanPath(cfg, false)} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("%s mode = %o, want 600", path, got)
		}
	}
}

func TestOceanDoctorDoesNotCreateStateDirectory(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "not-created")
	cfg := oceanConfig{
		project:  "buildmax-beta",
		vpc:      "buildmax-beta",
		bucket:   "buildmax-beta",
		region:   "sgp1",
		stateDir: stateDir,
	}
	_ = oceanDoctor(cfg)
	if exists(stateDir) {
		t.Fatalf("oceanDoctor created %s", stateDir)
	}
}

func pathHasSuffix(path, suffix string) bool {
	return strings.HasSuffix(filepath.Clean(path), filepath.Clean(suffix))
}
