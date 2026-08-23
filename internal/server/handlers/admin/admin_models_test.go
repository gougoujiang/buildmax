package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
)

const modelSecret = "sk-this-must-never-be-served"

func adminModelsMux(t *testing.T) (*http.ServeMux, *mock.MockLLMModelStore, *mock.MockAuditStore) {
	t.Helper()
	users := &mock.MockUserStore{}
	seedUser(t, users, adminUser, "admin@example.com")
	grants := &mock.MockSystemGrantStore{}
	grants.GrantForTest(adminUser, model.SystemRoleAdmin)

	models := &mock.MockLLMModelStore{}
	fast, err := models.CreateLLMModel(t.Context(), model.CreateLLMModelInput{
		Name: "Fast", ProviderType: "openai_compatible", APIURL: "https://example.test/v1",
		APIKey: modelSecret, Model: "provider/fast",
	})
	if err != nil {
		t.Fatalf("CreateLLMModel: %v", err)
	}
	if _, err := models.CreateLLMModel(t.Context(), model.CreateLLMModelInput{
		Name: "Deep", ProviderType: "openai_compatible", APIURL: "https://example.test/v1",
		APIKey: modelSecret, Model: "provider/deep",
	}); err != nil {
		t.Fatalf("CreateLLMModel: %v", err)
	}

	audits := &mock.MockAuditStore{}
	h := New(Config{
		JWTSecret:  testSecret,
		Grants:     grants,
		Users:      users,
		Teams:      &mock.MockTeamStore{},
		Models:     models,
		Audits:     audits,
		Audit:      audit.NewRecorder(audits),
		Deployment: DeploymentInfo{DefaultModel: fast.Name},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, models, audits
}

// TestAdminModelsNeverCarryACredential is the assertion this route exists
// under. The catalog is the one table in the system holding provider keys.
func TestAdminModelsNeverCarryACredential(t *testing.T) {
	mux, _, _ := adminModelsMux(t)
	rec := adminRequestAs(t, mux, adminCase{"GET", "/api/admin/llm/models"}, adminUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, modelSecret) || strings.Contains(strings.ToLower(body), "api_key") {
		t.Errorf("the catalog response carried a credential: %s", body)
	}
}

// TestAdminModelsReportWhichAreReachable: a model no alias points at cannot be
// called by any team however enabled it is, and that is the most common reason
// an operator's model "does not work".
func TestAdminModelsReportWhichAreReachable(t *testing.T) {
	mux, _, _ := adminModelsMux(t)
	rec := adminRequestAs(t, mux, adminCase{"GET", "/api/admin/llm/models"}, adminUser)
	var out AdminModelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.DefaultModel != "Fast" {
		t.Errorf("default_model = %q, want %q", out.DefaultModel, "Fast")
	}
	if len(out.Models) != 2 {
		t.Fatalf("models = %d, want 2", len(out.Models))
	}
}

func TestAdminModelEnableDisable(t *testing.T) {
	mux, models, audits := adminModelsMux(t)
	id := models.Models[0].ID

	rec := adminRequestAs(t, mux, adminCase{"POST", "/api/admin/llm/models/" + id + "/disable"}, adminUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable got %d: %s", rec.Code, rec.Body.String())
	}
	if models.Models[0].Enabled {
		t.Error("the model is still enabled")
	}
	if strings.Contains(rec.Body.String(), modelSecret) {
		t.Errorf("the toggle response carried a credential: %s", rec.Body.String())
	}

	if got := adminRequestAs(t, mux, adminCase{"POST", "/api/admin/llm/models/" + id + "/enable"}, adminUser).Code; got != http.StatusOK {
		t.Fatalf("enable got %d", got)
	}
	if !models.Models[0].Enabled {
		t.Error("the model was not re-enabled")
	}

	// The same actions the operator command writes: the trail does not
	// distinguish a catalog change by where it was made, only by who made it.
	want := []string{model.AuditModelDisabled, model.AuditModelEnabled}
	if len(audits.Events) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(audits.Events), audits.Events)
	}
	for i, action := range want {
		e := audits.Events[i]
		if e.Action != action || e.ActorType != model.AuditActorUser || e.ActorID != adminUser {
			t.Errorf("event %d = %+v, want %s by the administrator", i, e, action)
		}
		if e.Detail != "Fast" {
			t.Errorf("event %d should name the model: %+v", i, e)
		}
	}
}

func TestAdminModelToggleOnAnUnknownModel(t *testing.T) {
	mux, _, _ := adminModelsMux(t)
	if got := adminRequestAs(t, mux, adminCase{"POST", "/api/admin/llm/models/lm_nobody/disable"}, adminUser).Code; got != http.StatusNotFound {
		t.Errorf("got %d, want 404", got)
	}
}

// TestAdminModelsWithoutACatalogIs503 rather than an empty list, which would
// read as "this deployment has no models" on a deployment with no database.
func TestAdminModelsWithoutACatalogIs503(t *testing.T) {
	users := &mock.MockUserStore{}
	seedUser(t, users, adminUser, "admin@example.com")
	grants := &mock.MockSystemGrantStore{}
	grants.GrantForTest(adminUser, model.SystemRoleAdmin)
	audits := &mock.MockAuditStore{}
	h := New(Config{
		JWTSecret: testSecret,
		Grants:    grants,
		Users:     users,
		Teams:     &mock.MockTeamStore{},
		Audits:    audits,
		Audit:     audit.NewRecorder(audits),
	})
	mux := http.NewServeMux()
	h.Register(mux)

	if got := adminRequestAs(t, mux, adminCase{"GET", "/api/admin/llm/models"}, adminUser).Code; got != http.StatusServiceUnavailable {
		t.Errorf("got %d, want 503", got)
	}
}
