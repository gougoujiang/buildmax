package taskrun

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

func testManaged() ManagedInference {
	return ManagedInference{ServerURL: "https://buildmax.example.com", RunToken: "run-token"}
}

func TestManagedInferenceEnabled(t *testing.T) {
	tests := []struct {
		name string
		in   ManagedInference
		want bool
	}{
		{"complete", testManaged(), true},
		{"no server", ManagedInference{RunToken: "run-token"}, false},
		{"no token", ManagedInference{ServerURL: "https://buildmax.example.com"}, false},
		{"direct mode", ManagedInference{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Enabled(); got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestTokenFuncRefusesAnotherServer is why the credential is not simply handed
// to agentapp as a string. A model entry is configuration and could name any
// host; a run token belongs to the deployment that minted it, and sending it
// anywhere else would leak it to whatever server the entry pointed at.
func TestTokenFuncRefusesAnotherServer(t *testing.T) {
	fn := testManaged().tokenFunc()
	if fn == nil {
		t.Fatal("a run with a credential produced no token func")
	}

	token, err := fn("https://buildmax.example.com")
	if err != nil || token != "run-token" {
		t.Errorf("token = %q, err = %v", token, err)
	}
	// A trailing slash is the same server, not a different one.
	if token, err := fn("https://buildmax.example.com/"); err != nil || token != "run-token" {
		t.Errorf("trailing slash: token = %q, err = %v", token, err)
	}
	if _, err := fn("https://elsewhere.example.com"); err == nil {
		t.Error("the run token was handed to another server")
	}
}

// TestDirectRunOffersNoManagedCredential is the invariant that keeps the two
// transports from substituting for one another: with no run token, a managed
// entry has nothing to authenticate with and must fail rather than find some
// other way to reach a provider.
func TestDirectRunOffersNoManagedCredential(t *testing.T) {
	if fn := (ManagedInference{}).tokenFunc(); fn != nil {
		t.Error("a direct-mode run produced a managed token func")
	}
	if got := managedRunScope(ManagedInference{}, "r_1"); got != "" {
		t.Errorf("managedRunScope = %q, want empty for a direct-mode run", got)
	}
	if got := managedRunScope(testManaged(), "r_1"); got != "r_1" {
		t.Errorf("managedRunScope = %q, want r_1", got)
	}
}

// TestRuntimeModelEntriesKeepsTheDeploymentDefault covers the deployment that
// names no worker model: the gateway resolves the empty name, so the entry has
// to reach the runtime anyway. Dropping it took the smoke stacks down with
// `model not found: ""` — the runtime had no models to choose from at all.
func TestRuntimeModelEntriesKeepsTheDeploymentDefault(t *testing.T) {
	entry := config.ModelEntry{Name: "deployment default", ContextWindow: 32000}

	got := runtimeModelEntries(entry, testManaged())
	if len(got) != 1 {
		t.Fatalf("runtimeModelEntries returned %d entries, want the managed entry", len(got))
	}
	if got[0].Name != "deployment default" || got[0].ContextWindow != 32000 {
		t.Errorf("entry = %+v; want the name and context window the run was given", got[0])
	}
}

func TestRuntimeModelEntriesForANamedModel(t *testing.T) {
	entry := config.ModelEntry{Model: "Fast", Name: "Fast"}

	for name, managed := range map[string]ManagedInference{"managed": testManaged(), "direct": {}} {
		t.Run(name, func(t *testing.T) {
			got := runtimeModelEntries(entry, managed)
			if len(got) != 1 || got[0].Model != "Fast" {
				t.Errorf("runtimeModelEntries = %+v, want the named model", got)
			}
		})
	}
}

// A direct run names its own model, so an empty one is a server with no model
// configured rather than a default to resolve elsewhere. Passing an unnamed
// entry would send the prompt nowhere; settings.yaml still gets its say.
func TestRuntimeModelEntriesDropsAnUnnamedDirectModel(t *testing.T) {
	if got := runtimeModelEntries(config.ModelEntry{}, ManagedInference{}); got != nil {
		t.Errorf("runtimeModelEntries = %+v, want no entries", got)
	}
}
