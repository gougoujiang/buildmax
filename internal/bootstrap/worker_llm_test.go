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
		LLM: config.ServerLLMConfig{
			DefaultAlias: "default",
			Aliases:      map[string]string{"default": "lm_1", "deep": "lm_2"},
		},
		Worker: config.ServerWorkerConfig{
			LLM: config.ServerWorkerLLMConfig{Transport: config.TransportBuildMax, Alias: "default"},
		},
	}
}

// TestValidateWorkerLLMRejectsUnservableConfig is the difference between config
// that parses and a deployment that works. Every case here loads cleanly today
// and then fails at each run's first model call, where the cause reads as a
// model outage rather than a configuration mistake.
func TestValidateWorkerLLMRejectsUnservableConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*config.ServerConfig)
		wantErr string
	}{
		{
			name:    "no aliases granted to anyone",
			mutate:  func(sc *config.ServerConfig) { sc.LLM.Aliases = nil },
			wantErr: "llm.aliases is empty",
		},
		{
			name:    "the worker's alias is not one a team may call",
			mutate:  func(sc *config.ServerConfig) { sc.Worker.LLM.Alias = "nonexistent" },
			wantErr: "not one of llm.aliases",
		},
		{
			name: "nothing says which alias a run should use",
			mutate: func(sc *config.ServerConfig) {
				sc.Worker.LLM.Alias = ""
				sc.LLM.DefaultAlias = ""
			},
			wantErr: "does not say which",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sc := managedServerConfig()
			tc.mutate(&sc)
			err := validateWorkerLLM(sc)
			if err == nil {
				t.Fatal("the server started with a managed worker configuration it cannot serve")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateWorkerLLMAcceptsWorkableConfig(t *testing.T) {
	tests := []struct {
		name string
		sc   config.ServerConfig
	}{
		{"a named alias", managedServerConfig()},
		{"falling back to the default alias", func() config.ServerConfig {
			sc := managedServerConfig()
			sc.Worker.LLM.Alias = ""
			return sc
		}()},
		{"one alias and no default named", func() config.ServerConfig {
			sc := managedServerConfig()
			sc.Worker.LLM.Alias = ""
			sc.LLM.DefaultAlias = ""
			sc.LLM.Aliases = map[string]string{"only": "lm_1"}
			return sc
		}()},
		// Direct mode is not validated against the gateway at all: it never
		// touches it.
		{"direct mode with no gateway configured", config.ServerConfig{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateWorkerLLM(tc.sc); err != nil {
				t.Errorf("validateWorkerLLM: %v", err)
			}
		})
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

// TestWorkerLLMDescriptorTellsARunTheAliasAndNothingElse is the boundary the
// descriptor exists to hold: a worker learns which alias to name, never where
// the model lives or what pays for it.
func TestWorkerLLMDescriptorTellsARunTheAliasAndNothingElse(t *testing.T) {
	if got := workerLLMDescriptor(config.ServerWorkerLLMConfig{}); got != nil {
		t.Errorf("direct mode produced a descriptor: %+v", got)
	}

	got := workerLLMDescriptor(config.ServerWorkerLLMConfig{
		Transport:     config.TransportBuildMax,
		Alias:         "deep",
		ContextWindow: 128000,
		CallTimeout:   300,
	})
	if got == nil {
		t.Fatal("managed mode produced no descriptor")
	}
	want := workerclient.TaskRunLLM{
		Transport:     config.TransportBuildMax,
		Alias:         "deep",
		ContextWindow: 128000,
		CallTimeout:   300,
	}
	if *got != want {
		t.Errorf("descriptor = %+v, want %+v", *got, want)
	}
}

// TestResolveRunModelManagedCarriesNoProviderCredential is the outcome the whole
// change is for: a managed run holds an alias and a run token, and no upstream
// key.
func TestResolveRunModelManagedCarriesNoProviderCredential(t *testing.T) {
	sc := serverConfigWithDirectModel()
	llm := &workerclient.TaskRunLLM{Transport: config.TransportBuildMax, Alias: "deep", ContextWindow: 128000}

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
	if !entry.IsManaged() || entry.Model != "deep" {
		t.Errorf("entry = %+v, want a managed entry naming the alias", entry)
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
	llm := &workerclient.TaskRunLLM{Transport: config.TransportBuildMax, Alias: "deep"}

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
		if entry.IsManaged() {
			t.Errorf("descriptor %+v produced a managed entry", llm)
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
