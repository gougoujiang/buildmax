package agentdef

import "testing"

func TestSecretConsumption_EqualIgnoresOrder(t *testing.T) {
	a := SecretConsumption{Env: []SecretEnvGrant{
		{Secret: "sec_b", Item: "token", EnvName: "B"},
		{Secret: "sec_a", Item: "token", EnvName: "A"},
	}}
	b := SecretConsumption{Env: []SecretEnvGrant{
		{Secret: "sec_a", Item: "token", EnvName: "A"},
		{Secret: "sec_b", Item: "token", EnvName: "B"},
	}}
	if !a.Equal(b) {
		t.Fatal("same grants in a different order should compare equal")
	}
}

func TestSecretConsumption_EqualDetectsDifference(t *testing.T) {
	a := SecretConsumption{Env: []SecretEnvGrant{{Secret: "s", Item: "token", EnvName: "A"}}}
	b := SecretConsumption{Env: []SecretEnvGrant{{Secret: "s", Item: "token", EnvName: "B"}}}
	if a.Equal(b) {
		t.Fatal("different env names should not compare equal")
	}
	if a.Equal(SecretConsumption{}) {
		t.Fatal("a grant should not equal empty")
	}
	if !(SecretConsumption{}).Equal(SecretConsumption{}) {
		t.Fatal("two empties should compare equal")
	}
}

func TestSecretConsumption_CanonicalIsStable(t *testing.T) {
	c := SecretConsumption{Env: []SecretEnvGrant{
		{Secret: "sec_b", Item: "token", EnvName: "B"},
		{Secret: "sec_a", Item: "z", EnvName: "Z"},
		{Secret: "sec_a", Item: "a", EnvName: "A"},
	}}
	got := c.Canonical().Env
	if got[0].Secret != "sec_a" || got[0].Item != "a" || got[2].Secret != "sec_b" {
		t.Fatalf("canonical order = %+v", got)
	}
	// Canonical of canonical is identical.
	if !c.Canonical().Equal(c.Canonical().Canonical()) {
		t.Fatal("canonical is not idempotent")
	}
}

func TestIsEnvName(t *testing.T) {
	valid := []string{"HOME", "GH_TOKEN", "_x", "A1"}
	invalid := []string{"", "1A", "GH-TOKEN", "a b", "AWS.REGION"}
	for _, v := range valid {
		if !IsEnvName(v) {
			t.Errorf("IsEnvName(%q) = false, want true", v)
		}
	}
	for _, v := range invalid {
		if IsEnvName(v) {
			t.Errorf("IsEnvName(%q) = true, want false", v)
		}
	}
}

func TestSecretEnvGrant_WholeGroup(t *testing.T) {
	if !(SecretEnvGrant{Secret: "s"}).WholeGroup() {
		t.Fatal("empty item should be a whole-group grant")
	}
	if (SecretEnvGrant{Secret: "s", Item: "token"}).WholeGroup() {
		t.Fatal("a named item is not a whole-group grant")
	}
}
