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
		"BUILDMAX_OCEAN_DATABASE_VERSION",
	} {
		t.Setenv(name, "")
	}

	cfg, err := loadOceanConfig()
	if err != nil {
		t.Fatalf("loadOceanConfig: %v", err)
	}
	if cfg.project != "buildmax-beta" || cfg.vpc != "buildmax-beta" || cfg.bucket != "buildmax-beta" || cfg.region != "sgp1" || cfg.databaseVersion != "8.4" {
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
	t.Setenv("BUILDMAX_OCEAN_DATABASE_VERSION", "9.0")

	cfg, err := loadOceanConfig()
	if err != nil {
		t.Fatalf("loadOceanConfig: %v", err)
	}
	if cfg.project != "project-x" || cfg.vpc != "vpc-x" || cfg.bucket != "bucket-x" || cfg.region != "nyc3" || cfg.databaseVersion != "9.0" {
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
		project:         "buildmax-beta",
		vpc:             "buildmax-beta",
		bucket:          "buildmax-beta",
		region:          "sgp1",
		databaseVersion: "8.4",
		stateDir:        stateDir,
	}
	_ = oceanDoctor(cfg)
	if exists(stateDir) {
		t.Fatalf("oceanDoctor created %s", stateDir)
	}
}

func TestOceanApplicationConfigRequiresPrivateAccessBoundary(t *testing.T) {
	t.Setenv("BUILDMAX_OCEAN_HOSTNAME", "buildmax.beta.cloudbb.io")
	t.Setenv("BUILDMAX_OCEAN_ALLOWED_CIDRS", "")
	if _, err := loadOceanApplicationConfig(); err == nil {
		t.Fatal("loadOceanApplicationConfig accepted a public deployment with no allow-list")
	}
}

func TestOceanApplicationConfigPinsImagesAndNormalizesCIDRs(t *testing.T) {
	t.Setenv("BUILDMAX_OCEAN_HOSTNAME", "BuildMax.Beta.CloudBB.io")
	t.Setenv("BUILDMAX_OCEAN_ALLOWED_CIDRS", "203.0.113.7/32, 2001:db8::1/128")
	t.Setenv("BUILDMAX_OCEAN_IMAGE", "example/buildmax@sha256:"+strings.Repeat("a", 64))
	t.Setenv("BUILDMAX_OCEAN_PORTAL_IMAGE", "example/portal@sha256:"+strings.Repeat("b", 64))
	t.Setenv("BUILDMAX_OCEAN_EDGE_IMAGE", "example/edge@sha256:"+strings.Repeat("c", 64))

	cfg, err := loadOceanApplicationConfig()
	if err != nil {
		t.Fatalf("loadOceanApplicationConfig: %v", err)
	}
	if cfg.hostname != "buildmax.beta.cloudbb.io" {
		t.Errorf("hostname = %q", cfg.hostname)
	}
	if got := strings.Join(cfg.allowedCIDRs, ","); got != "203.0.113.7/32,2001:db8::1/128" {
		t.Errorf("allowed CIDRs = %q", got)
	}
	if cfg.buildmaxImage != "example/buildmax@sha256:"+strings.Repeat("a", 64) {
		t.Errorf("image = %q", cfg.buildmaxImage)
	}
}

func TestOceanApplicationConfigRejectsMutableImageTags(t *testing.T) {
	t.Setenv("BUILDMAX_OCEAN_HOSTNAME", "buildmax.beta.cloudbb.io")
	t.Setenv("BUILDMAX_OCEAN_ALLOWED_CIDRS", "203.0.113.7/32")
	t.Setenv("BUILDMAX_OCEAN_IMAGE", "ghcr.io/example/buildmax:latest")
	if _, err := loadOceanApplicationConfig(); err == nil {
		t.Fatal("loadOceanApplicationConfig accepted a mutable image tag")
	}
}

func TestOceanCaddyfileRestrictsApplicationAndRoutesOneOrigin(t *testing.T) {
	got := oceanCaddyfile(oceanApplicationConfig{
		hostname:     "buildmax.beta.cloudbb.io",
		allowedCIDRs: []string{"203.0.113.7/32"},
	})
	for _, want := range []string{
		"buildmax.beta.cloudbb.io",
		"not remote_ip 203.0.113.7/32",
		"reverse_proxy buildmax:5678",
		"reverse_proxy buildmax-portal:80",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Caddyfile missing %q:\n%s", want, got)
		}
	}
}

func TestOceanManifestUsesOnlyPinnedImages(t *testing.T) {
	app := oceanApplicationConfig{
		buildmaxImage: "example/buildmax@sha256:" + strings.Repeat("a", 64),
		portalImage:   "example/portal@sha256:" + strings.Repeat("b", 64),
		edgeImage:     "example/edge@sha256:" + strings.Repeat("c", 64),
	}
	manifest, err := renderOceanManifest(app)
	if err != nil {
		t.Fatal(err)
	}
	text := string(manifest)
	for _, want := range []string{app.buildmaxImage, app.portalImage, app.edgeImage, "externalTrafficPolicy: Local"} {
		if !strings.Contains(text, want) {
			t.Errorf("manifest missing %q", want)
		}
	}
	if strings.Contains(text, "{{") {
		t.Error("manifest contains an unresolved template expression")
	}
}

func pathHasSuffix(path, suffix string) bool {
	return strings.HasSuffix(filepath.Clean(path), filepath.Clean(suffix))
}
