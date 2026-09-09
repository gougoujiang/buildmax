package main

import "testing"

func TestEphemeralKindRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, ok := readEphemeralKind(); ok {
		t.Fatal("a fresh worktree should have no ephemeral cluster record")
	}

	want := ephemeralKind{cluster: "buildmax-eph-abcd1234", portalPort: "18080", tlsPort: "18443"}
	if err := writeEphemeralKind(want); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, ok := readEphemeralKind()
	if !ok || got != want {
		t.Fatalf("roundtrip: got %+v ok=%v, want %+v", got, ok, want)
	}

	if err := clearEphemeralKind(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, ok := readEphemeralKind(); ok {
		t.Fatal("record should be gone after clear")
	}
	// Clearing an absent record is not an error, so a second `kind down` is safe.
	if err := clearEphemeralKind(); err != nil {
		t.Fatalf("clear when absent: %v", err)
	}
}

// A record missing any field is treated as none, so a half-written file never
// points some commands at a cluster and others at the default.
func TestEphemeralKindIgnoresIncompleteRecord(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := writeEphemeralKind(ephemeralKind{cluster: "buildmax-eph-x", portalPort: "18080"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, ok := readEphemeralKind(); ok {
		t.Fatal("a record with no tls_port should read as none")
	}
}

func TestKindIdentityPrecedence(t *testing.T) {
	t.Chdir(t.TempDir())
	// Clear any inherited overrides so the default case is really the default.
	t.Setenv("BUILDMAX_KIND_CLUSTER", "")
	t.Setenv("BUILDMAX_KIND_PORTAL_PORT", "")
	t.Setenv("BUILDMAX_KIND_TLS_PORT", "")

	if got := kindClusterName(); got != defaultKindCluster {
		t.Errorf("no record: cluster = %q, want %q", got, defaultKindCluster)
	}
	if got := kindPortalPort(); got != defaultKindPortalPort {
		t.Errorf("no record: portal port = %q, want %q", got, defaultKindPortalPort)
	}

	if err := writeEphemeralKind(ephemeralKind{cluster: "buildmax-eph-xy", portalPort: "18080", tlsPort: "18443"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := kindClusterName(); got != "buildmax-eph-xy" {
		t.Errorf("with record: cluster = %q, want the ephemeral name", got)
	}
	if got := kindPortalPort(); got != "18080" {
		t.Errorf("with record: portal port = %q, want the ephemeral port", got)
	}

	// An explicit environment variable overrides the ephemeral record.
	t.Setenv("BUILDMAX_KIND_CLUSTER", "manual")
	if got := kindClusterName(); got != "manual" {
		t.Errorf("explicit env should win over the record: cluster = %q, want %q", got, "manual")
	}
}
