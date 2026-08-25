package team

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/infra/pluginwire"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

const activationSecret = "activation-test-secret"

type activationFixture struct {
	mux     *http.ServeMux
	catalog *mock.MockPluginStore
	teams   *mock.MockTeamStore
	base    string
}

// newActivationFixture wires the real handler over in-memory stores. The team
// has an owner, an ordinary member, and a non-member, because the authority
// split is the thing these routes are for.
func newActivationFixture(t *testing.T, curation coreplugin.Curation) *activationFixture {
	t.Helper()
	teamID := "tm_1"
	teams := &mock.MockTeamStore{
		Teams: []coreteam.Team{{ID: teamID, Name: "Team", CreatedBy: "u_owner", PluginCuration: curation}},
		Members: []coreteam.Member{
			{TeamID: teamID, UserID: "u_owner", Role: coreteam.RoleOwner},
			{TeamID: teamID, UserID: "u_member", Role: coreteam.RoleMember},
		},
	}
	catalog := mock.NewMockPluginStore()
	svc := &pluginsvc.Service{
		Catalog:     catalog,
		Activations: mock.NewMockPluginActivationStore(),
		Teams:       teams,
		Audit:       audit.NewRecorder(&mock.MockAuditStore{}),
	}
	h := New(Config{JWTSecret: activationSecret, Teams: teams, Plugins: svc})
	mux := http.NewServeMux()
	h.Register(mux)
	return &activationFixture{mux: mux, catalog: catalog, teams: teams, base: "/api/teams/" + teamID}
}

func (f *activationFixture) publish(t *testing.T, name, version string, in coreplugin.Inspection) {
	t.Helper()
	ctx := context.Background()
	if entry, err := f.catalog.GetPlugin(ctx, name); err != nil {
		t.Fatalf("GetPlugin: %v", err)
	} else if entry == nil {
		if _, err := f.catalog.CreatePlugin(ctx, coreplugin.CreateInput{Name: name, CreatedBy: "u_owner"}); err != nil {
			t.Fatalf("CreatePlugin: %v", err)
		}
	}
	if _, err := f.catalog.CreatePluginRelease(ctx, coreplugin.CreateReleaseInput{
		PluginName: name, Version: version, Digest: "sha256:" + version,
		ObjectKey: "bm/" + name, Inspection: in, PublishedBy: "u_owner",
	}); err != nil {
		t.Fatalf("CreatePluginRelease: %v", err)
	}
}

func (f *activationFixture) call(t *testing.T, method, userID, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, f.base+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, activationSecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func TestActivationRoutesActivateAndList(t *testing.T) {
	f := newActivationFixture(t, coreplugin.CurationCurated)
	f.publish(t, "code-review", "1.2.0", coreplugin.Inspection{Skills: []string{"review"}})

	rec := f.call(t, http.MethodPost, "u_owner", "/plugin-activations", `{"plugin_name":"code-review"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("activate status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	rec = f.call(t, http.MethodGet, "u_member", "/plugin-activations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got pluginwire.ActivationsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Curation != coreplugin.CurationCurated {
		t.Errorf("curation = %q, want curated", got.Curation)
	}
	if len(got.Activations) != 1 || got.Activations[0].Version != "1.2.0" {
		t.Fatalf("activations = %+v, want one pinned to 1.2.0", got.Activations)
	}
	if got.Activations[0].Digest == "" {
		t.Error("the response dropped the digest, which is half the pin")
	}
}

// Reading is any member's question; changing is not.
func TestActivationRoutesAuthority(t *testing.T) {
	f := newActivationFixture(t, coreplugin.CurationCurated)
	f.publish(t, "code-review", "1.0.0", coreplugin.Inspection{Skills: []string{"review"}})

	if rec := f.call(t, http.MethodGet, "u_member", "/plugin-activations", ""); rec.Code != http.StatusOK {
		t.Errorf("member read status = %d, want 200", rec.Code)
	}
	rec := f.call(t, http.MethodPost, "u_member", "/plugin-activations", `{"plugin_name":"code-review"}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("member activate status = %d, want 403", rec.Code)
	}
	rec = f.call(t, http.MethodPut, "u_member", "/plugin-curation", `{"curation":"open"}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("member curation status = %d, want 403", rec.Code)
	}
	rec = f.call(t, http.MethodGet, "u_outsider", "/plugin-activations", "")
	if rec.Code == http.StatusOK {
		t.Errorf("a non-member read a team's activations (status %d)", rec.Code)
	}
}

func TestActivationRoutesRefuseExecutableContent(t *testing.T) {
	f := newActivationFixture(t, coreplugin.CurationCurated)
	f.publish(t, "guard", "1.0.0", coreplugin.Inspection{Hooks: []coreplugin.Hook{{Event: "pre_tool_use"}}})

	rec := f.call(t, http.MethodPost, "u_owner", "/plugin-activations", `{"plugin_name":"guard"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "hooks") {
		t.Errorf("the refusal must say what is wrong, got %q", rec.Body.String())
	}
}

// A second activation is a pin move, and saying so beats a 500.
func TestActivatingTwiceIsAConflict(t *testing.T) {
	f := newActivationFixture(t, coreplugin.CurationCurated)
	f.publish(t, "code-review", "1.0.0", coreplugin.Inspection{Skills: []string{"review"}})

	if rec := f.call(t, http.MethodPost, "u_owner", "/plugin-activations", `{"plugin_name":"code-review"}`); rec.Code != http.StatusCreated {
		t.Fatalf("first activate status = %d", rec.Code)
	}
	rec := f.call(t, http.MethodPost, "u_owner", "/plugin-activations", `{"plugin_name":"code-review"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second activate status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchMovesThePinAndSuspends(t *testing.T) {
	f := newActivationFixture(t, coreplugin.CurationCurated)
	f.publish(t, "code-review", "1.0.0", coreplugin.Inspection{Skills: []string{"review"}})
	f.publish(t, "code-review", "2.0.0", coreplugin.Inspection{Skills: []string{"review"}})
	if rec := f.call(t, http.MethodPost, "u_owner", "/plugin-activations", `{"plugin_name":"code-review","version":"1.0.0"}`); rec.Code != http.StatusCreated {
		t.Fatalf("activate status = %d", rec.Code)
	}

	rec := f.call(t, http.MethodPatch, "u_owner", "/plugin-activations/code-review", `{"version":"2.0.0"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("move status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var moved coreplugin.Activation
	if err := json.Unmarshal(rec.Body.Bytes(), &moved); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if moved.Version != "2.0.0" {
		t.Errorf("version = %q, want 2.0.0", moved.Version)
	}

	rec = f.call(t, http.MethodPatch, "u_owner", "/plugin-activations/code-review", `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("suspend status = %d: %s", rec.Code, rec.Body.String())
	}
	var suspended coreplugin.Activation
	if err := json.Unmarshal(rec.Body.Bytes(), &suspended); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if suspended.Enabled || suspended.Version != "2.0.0" {
		t.Errorf("suspension lost the pin: %+v", suspended)
	}
}

// A refused pin move must leave the activation exactly as it was.
func TestARefusedPinMoveChangesNothing(t *testing.T) {
	f := newActivationFixture(t, coreplugin.CurationCurated)
	f.publish(t, "code-review", "1.0.0", coreplugin.Inspection{Skills: []string{"review"}})
	f.publish(t, "code-review", "1.1.0", coreplugin.Inspection{Hooks: []coreplugin.Hook{{Event: "pre_tool_use"}}})
	if rec := f.call(t, http.MethodPost, "u_owner", "/plugin-activations", `{"plugin_name":"code-review","version":"1.0.0"}`); rec.Code != http.StatusCreated {
		t.Fatalf("activate status = %d", rec.Code)
	}

	rec := f.call(t, http.MethodPatch, "u_owner", "/plugin-activations/code-review",
		`{"version":"1.1.0","enabled":false}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
	rec = f.call(t, http.MethodGet, "u_owner", "/plugin-activations", "")
	var got pluginwire.ActivationsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Activations[0].Version != "1.0.0" || !got.Activations[0].Enabled {
		t.Errorf("a refused request changed the activation: %+v", got.Activations[0])
	}
}

func TestPatchWithNoFieldsIsRefused(t *testing.T) {
	f := newActivationFixture(t, coreplugin.CurationCurated)
	f.publish(t, "code-review", "1.0.0", coreplugin.Inspection{Skills: []string{"review"}})
	if rec := f.call(t, http.MethodPost, "u_owner", "/plugin-activations", `{"plugin_name":"code-review"}`); rec.Code != http.StatusCreated {
		t.Fatalf("activate status = %d", rec.Code)
	}
	rec := f.call(t, http.MethodPatch, "u_owner", "/plugin-activations/code-review", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestSetCurationRoundTripsAndValidates(t *testing.T) {
	f := newActivationFixture(t, coreplugin.CurationOpen)

	rec := f.call(t, http.MethodPut, "u_owner", "/plugin-curation", `{"curation":"curated"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	rec = f.call(t, http.MethodGet, "u_owner", "/plugin-activations", "")
	var got pluginwire.ActivationsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Curation != coreplugin.CurationCurated {
		t.Errorf("curation = %q, want curated", got.Curation)
	}

	rec = f.call(t, http.MethodPut, "u_owner", "/plugin-curation", `{"curation":"whatever"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 for an unknown mode", rec.Code)
	}
}

// A deployment without a Marketplace answers rather than panicking.
func TestActivationRoutesWithoutTheService(t *testing.T) {
	teamID := "tm_1"
	teams := &mock.MockTeamStore{
		Teams:   []coreteam.Team{{ID: teamID, Name: "Team", CreatedBy: "u_owner"}},
		Members: []coreteam.Member{{TeamID: teamID, UserID: "u_owner", Role: coreteam.RoleOwner}},
	}
	h := New(Config{JWTSecret: activationSecret, Teams: teams})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/plugin-activations", nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u_owner", activationSecret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
