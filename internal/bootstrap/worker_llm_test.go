package bootstrap

import (
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/server/authtoken"
)

func managedServerConfig() config.ServerConfig {
	return config.ServerConfig{
		LLM: config.ServerLLMConfig{DefaultModel: "Fast"},
		Worker: config.ServerWorkerConfig{
			LLM: config.ServerWorkerLLMConfig{Transport: config.TransportBuildMax, Model: "Fast"},
		},
	}
}

// TestEveryRunGetsAToken records that the credential is no longer tied to
// managed inference. It started as a gateway credential, but it is now what a
// worker presents on all of its own routes, so a direct-mode run needs one to
// report the work it did.
func TestEveryRunGetsAToken(t *testing.T) {
	for name, sc := range map[string]config.ServerConfig{
		"direct":  {},
		"managed": managedServerConfig(),
	} {
		if mint := runTokenMinter(sc, "secret"); mint == nil {
			t.Errorf("a %s deployment mints no run tokens", name)
		}
	}

	mint := runTokenMinter(managedServerConfig(), "secret")
	claims := authtoken.RunClaims{UserID: "u_1", TeamID: "tm_1", TaskRunID: "r_1", TaskID: "t_1"}
	token, err := mint(claims)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	got, ok := authtoken.ParseRun(token, "secret")
	if !ok || got != claims {
		t.Errorf("ParseRun = %+v, ok = %v", got, ok)
	}
}

// TestWorkerLLMDescriptorTellsARunTheModelAndNothingElse is the boundary the
// descriptor exists to hold: a worker learns which model to name, never where
// it lives or what pays for it.
func TestWorkerLLMDescriptorTellsARunTheModelAndNothingElse(t *testing.T) {
	if got := workerLLMDescriptor(config.ServerWorkerLLMConfig{}); got != nil {
		t.Errorf("direct mode produced a descriptor: %+v", got)
	}

	got := workerLLMDescriptor(config.ServerWorkerLLMConfig{
		Transport:     config.TransportBuildMax,
		Model:         "Deep",
		ContextWindow: 128000,
		CallTimeout:   300,
	})
	if got == nil {
		t.Fatal("managed mode produced no descriptor")
	}
	want := workerclient.TaskRunLLM{
		Transport:     config.TransportBuildMax,
		Model:         "Deep",
		ContextWindow: 128000,
		CallTimeout:   300,
	}
	if *got != want {
		t.Errorf("descriptor = %+v, want %+v", *got, want)
	}
}

// TestResolveRunModelManagedCarriesNoProviderCredential is the outcome the whole
// change is for: a managed run holds a model name and a run token, and no
// upstream key.
func TestResolveRunModelManagedCarriesNoProviderCredential(t *testing.T) {
	sc := serverConfigWithDirectModel()
	llm := &workerclient.TaskRunLLM{Transport: config.TransportBuildMax, Model: "Deep", ContextWindow: 128000}

	entry, managed, err := resolveRunModel(sc, llm, "https://buildmax.example.com", "run-token")
	if err != nil {
		t.Fatalf("resolveRunModel: %v", err)
	}
	if entry.APIKey != "" {
		t.Errorf("a managed run was given a provider key: %q", entry.APIKey)
	}
	if entry.APIURL != "" {
		t.Errorf("a managed run was given a provider endpoint: %q", entry.APIURL)
	}
	if entry.Model != "Deep" {
		t.Errorf("entry = %+v, want an entry naming the model", entry)
	}
	if !managed.Enabled() || managed.RunToken != "run-token" {
		t.Errorf("managed = %+v", managed)
	}
}

// TestResolveRunModelManagedWithoutATokenFails keeps the two transports from
// substituting for one another. A run that cannot authenticate to the gateway
// must stop, not reach for whatever provider credential is around.
func TestResolveRunModelManagedWithoutATokenFails(t *testing.T) {
	sc := serverConfigWithDirectModel()
	llm := &workerclient.TaskRunLLM{Transport: config.TransportBuildMax, Model: "Deep"}

	_, _, err := resolveRunModel(sc, llm, "https://buildmax.example.com", "")
	if err == nil {
		t.Fatal("a managed run started with no gateway credential")
	}
	if !strings.Contains(err.Error(), config.EnvKeyBuildmaxRunToken) {
		t.Errorf("error %q does not name the missing credential", err)
	}
}

// TestResolveRunModelDirectIsUnchanged covers every deployment that exists
// today: no descriptor, or one that says direct, still runs the server's own
// model with the server's own key.
func TestResolveRunModelDirectIsUnchanged(t *testing.T) {
	sc := serverConfigWithDirectModel()

	for _, llm := range []*workerclient.TaskRunLLM{nil, {Transport: config.TransportDirect}} {
		entry, managed, err := resolveRunModel(sc, llm, "https://buildmax.example.com", "run-token")
		if err != nil {
			t.Fatalf("resolveRunModel: %v", err)
		}
		if entry.APIKey != "provider-key" || entry.Model != "openai/gpt-4o" {
			t.Errorf("entry = %+v, want the server's own model", entry)
		}
		// A direct run gets no gateway credential even when one was delivered:
		// it has nothing to present it to.
		if managed.Enabled() {
			t.Errorf("a direct run carries managed inference: %+v", managed)
		}
	}
}

func serverConfigWithDirectModel() config.ServerConfig {
	return config.ServerConfig{
		Conversation: config.ServerConvConfig{
			Model: config.ServerModelEntry{
				Model:  "openai/gpt-4o",
				Name:   "GPT-4o",
				APIURL: "https://openrouter.ai/api/v1",
				APIKey: "provider-key",
			},
		},
	}
}
