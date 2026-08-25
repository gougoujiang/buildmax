package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
	"github.com/gougoujiang/buildmax/internal/core/model"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
)

// stubSchemaStore reports a fixed migration history.
type stubSchemaStore struct {
	migrations []model.SchemaMigration
	err        error
}

func (s stubSchemaStore) AppliedMigrations(_ context.Context) ([]model.SchemaMigration, error) {
	return s.migrations, s.err
}

func systemMux(t *testing.T, probes []DependencyProbe, redacted any) *http.ServeMux {
	t.Helper()
	users := &mock.MockUserStore{}
	seedUser(t, users, adminUser, "admin@example.com")
	grants := &mock.MockSystemGrantStore{}
	grants.GrantForTest(adminUser, coreidentity.SystemRoleAdmin)

	runs := &mock.MockTaskRunStore{Runs: []coretask.Run{
		{ID: "r_1", Status: "SUCCEEDED"},
		{ID: "r_2", Status: "SUCCEEDED"},
		{ID: "r_3", Status: "PENDING"},
	}}
	audits := &mock.MockAuditStore{}
	h := New(Config{
		JWTSecret: testSecret,
		Grants:    grants,
		Users:     users,
		Teams:     &mock.MockTeamStore{},
		TaskRuns:  runs,
		Schema:    stubSchemaStore{migrations: []model.SchemaMigration{{ID: "0001_first", AppliedAt: time.Unix(100, 0).UTC()}}},
		Audits:    audits,
		Audit:     audit.NewRecorder(audits),
		Deployment: DeploymentInfo{
			Version:            "1.2.3 (abcdef)",
			WorkerRunMode:      "k8s_job",
			WorkerLLMTransport: "buildmax",
			AllowSignup:        false,
		},
		DependencyProbes: probes,
		RedactedConfig:   redacted,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func getAs(t *testing.T, mux *http.ServeMux, path, userID string) (int, string) {
	t.Helper()
	rec := adminRequestAs(t, mux, adminCase{"GET", path}, userID)
	return rec.Code, rec.Body.String()
}

func TestAdminSystemStatus(t *testing.T) {
	mux := systemMux(t, []DependencyProbe{
		{Name: "database", Probe: func(context.Context) error { return nil }},
		{Name: "object_store", Probe: func(context.Context) error { return errors.New("bucket unreachable: s3://secrets@host/bucket") }},
	}, nil)

	code, body := getAs(t, mux, "/api/admin/system", adminUser)
	if code != http.StatusOK {
		t.Fatalf("got %d: %s", code, body)
	}
	var got AdminSystemResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Version != "1.2.3 (abcdef)" {
		t.Errorf("version = %q", got.Version)
	}
	if got.Ready {
		t.Error("a failed dependency should make the deployment not ready")
	}
	if len(got.Dependencies) != 2 || got.Dependencies[0].Status != "ok" || got.Dependencies[1].Status != "failed" {
		t.Errorf("dependencies = %+v", got.Dependencies)
	}
	// The probe's error names a bucket and a credential. The status endpoint
	// says which check failed and never why — the reason goes to the log,
	// where an operator already has to be.
	if strings.Contains(body, "bucket unreachable") || strings.Contains(body, "secrets@host") {
		t.Errorf("the probe's error text reached the response: %s", body)
	}
	if got.TaskRuns["SUCCEEDED"] != 2 || got.TaskRuns["PENDING"] != 1 {
		t.Errorf("task_runs = %v", got.TaskRuns)
	}
	if got.SystemAdmins != 1 {
		t.Errorf("system_admins = %d, want 1", got.SystemAdmins)
	}
	if len(got.SchemaMigrations) != 1 || got.SchemaMigrations[0].ID != "0001_first" {
		t.Errorf("schema_migrations = %+v", got.SchemaMigrations)
	}
	if got.SandboxSurface != "" {
		t.Errorf("sandbox_surface = %q; no worker path passes one, and claiming a boundary that is not applied is worse than claiming none", got.SandboxSurface)
	}
}

// TestAdminSystemStatusSurvivesAPartialOutage: a status page that 500s because
// one of its five questions could not be answered tells an operator nothing
// during exactly the outage they opened it for.
func TestAdminSystemStatusSurvivesAPartialOutage(t *testing.T) {
	users := &mock.MockUserStore{}
	seedUser(t, users, adminUser, "admin@example.com")
	grants := &mock.MockSystemGrantStore{}
	grants.GrantForTest(adminUser, coreidentity.SystemRoleAdmin)
	audits := &mock.MockAuditStore{}
	h := New(Config{
		JWTSecret:  testSecret,
		Grants:     grants,
		Users:      users,
		Teams:      &mock.MockTeamStore{},
		Schema:     stubSchemaStore{err: errors.New("database is gone")},
		Audits:     audits,
		Audit:      audit.NewRecorder(audits),
		Deployment: DeploymentInfo{Version: "dev"},
	})
	mux := http.NewServeMux()
	h.Register(mux)

	code, body := getAs(t, mux, "/api/admin/system", adminUser)
	if code != http.StatusOK {
		t.Fatalf("got %d, want 200 with the parts that could be answered: %s", code, body)
	}
	var got AdminSystemResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Version != "dev" {
		t.Errorf("the facts that were available should still be reported: %+v", got)
	}
	// Empty collections serialize as [] and {}, not null: a Portal reading
	// this should not have to branch on nil.
	for _, want := range []string{`"schema_migrations":[]`, `"dependencies":[]`, `"task_runs":{}`} {
		if !strings.Contains(body, want) {
			t.Errorf("body should contain %s, got %s", want, body)
		}
	}
}

// stubRedactedConfig stands in for what internal/config produces.
//
// The handler is given a view and serves it; whether that view is safe is
// internal/config's responsibility and is tested there, against the real
// ServerConfig. This package cannot import config — the architecture test
// forbids it — and duplicating the assertion here with a hand-built struct
// would prove nothing about the real one anyway.
type stubRedactedConfig struct {
	Port      int              `json:"port"`
	JWTSecret stubSecretStatus `json:"jwt_secret"`
	Warnings  []string         `json:"warnings"`
}

type stubSecretStatus struct {
	Set bool `json:"set"`
}

// TestAdminConfigServesTheViewItIsGiven, and adds nothing to it. The handler
// has no business reaching for configuration of its own.
func TestAdminConfigServesTheViewItIsGiven(t *testing.T) {
	view := stubRedactedConfig{
		Port:      5678,
		JWTSecret: stubSecretStatus{Set: true},
		Warnings:  []string{"allow_signup is on"},
	}
	mux := systemMux(t, nil, view)

	code, body := getAs(t, mux, "/api/admin/config", adminUser)
	if code != http.StatusOK {
		t.Fatalf("got %d: %s", code, body)
	}
	for _, want := range []string{`"port":5678`, `"jwt_secret":{"set":true}`, "allow_signup is on"} {
		if !strings.Contains(body, want) {
			t.Errorf("body should contain %s, got %s", want, body)
		}
	}
}

// TestAdminConfigWithoutAViewIs503 rather than an empty object, which would
// read as "there is no configuration".
func TestAdminConfigWithoutAViewIs503(t *testing.T) {
	mux := systemMux(t, nil, nil)
	if code, _ := getAs(t, mux, "/api/admin/config", adminUser); code != http.StatusServiceUnavailable {
		t.Errorf("got %d, want 503", code)
	}
}

// TestAdminSystemRoutesRefuseNonAdmins repeats the boundary at this surface,
// because these two are the routes that describe the deployment itself.
func TestAdminSystemRoutesRefuseNonAdmins(t *testing.T) {
	mux := systemMux(t, nil, stubRedactedConfig{})
	for _, path := range []string{"/api/admin/system", "/api/admin/config"} {
		if code, _ := getAs(t, mux, path, "u_nobody"); code != http.StatusForbidden {
			t.Errorf("%s for an ordinary user got %d, want 403", path, code)
		}
		if code, _ := getAs(t, mux, path, ""); code != http.StatusUnauthorized {
			t.Errorf("%s anonymous got %d, want 401", path, code)
		}
	}
}
