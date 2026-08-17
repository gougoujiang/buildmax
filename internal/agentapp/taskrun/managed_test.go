package taskrun

import (
	"testing"
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
