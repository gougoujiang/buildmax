package config

import "testing"

func TestResolvePermissions_ValidatesAndNormalizes(t *testing.T) {
	res := ResolvePermissions(ToolsConfig{Permissions: map[string]string{
		"Write":  " ALLOW ",
		"Bash":   "deny",
		"Read":   "bogus",
		"Edit":   "",
		"Task":   "Ask",
		"WebFet": "allow",
	}})
	if len(res.Entries) != 4 {
		t.Fatalf("entries = %d, want 4: %+v", len(res.Entries), res.Entries)
	}
	if len(res.Invalid) != 2 {
		t.Errorf("invalid = %v, want the bogus and empty actions", res.Invalid)
	}
	for _, e := range res.Entries {
		if e.Key != normalizeKey(e.Key) {
			t.Errorf("key %q was not normalized", e.Key)
		}
		if e.Display == "" {
			t.Errorf("entry %q lost the key as written", e.Key)
		}
	}
}

// TestLookup_CaseInsensitive guards the trap that made the first implementation
// silently do nothing: viper lowercases every map key it loads, so a rule
// written as "Write" arrives as "write" and would never match a tool name.
func TestLookup_CaseInsensitive(t *testing.T) {
	res := ResolvePermissions(ToolsConfig{Permissions: map[string]string{"write": "allow"}})
	e, ok := res.Lookup("Write", "")
	if !ok || e.Action != PermissionAllow {
		t.Fatalf("Lookup(Write) = (%+v, %v), want the allow rule", e, ok)
	}
}

func TestLookup_MostSpecificWins(t *testing.T) {
	res := ResolvePermissions(ToolsConfig{Permissions: map[string]string{
		"CallMcpTool":                    "deny",
		"CallMcpTool:github/*":           "ask",
		"CallMcpTool:github/get_issue":   "allow",
		"CallMcpTool:gitlab/delete_repo": "deny",
	}})
	for _, tc := range []struct {
		name, scope, want string
	}{
		{"CallMcpTool", "CallMcpTool:github/get_issue", PermissionAllow},  // exact
		{"CallMcpTool", "CallMcpTool:github/create_issue", PermissionAsk}, // prefix
		{"CallMcpTool", "CallMcpTool:jira/anything", PermissionDeny},      // falls back to the tool name
		{"CallMcpTool", "CallMcpTool:gitlab/delete_repo", PermissionDeny}, // exact beats the bare name
		{"Write", "", ""}, // unrelated tool: no rule
	} {
		e, ok := res.Lookup(tc.name, tc.scope)
		if tc.want == "" {
			if ok {
				t.Errorf("Lookup(%q, %q) matched %+v, want no rule", tc.name, tc.scope, e)
			}
			continue
		}
		if !ok || e.Action != tc.want {
			t.Errorf("Lookup(%q, %q) = (%v, %v), want %q", tc.name, tc.scope, e.Action, ok, tc.want)
		}
	}
}

func TestLookup_EmptyResolutionMatchesNothing(t *testing.T) {
	if _, ok := (PermissionResolution{}).Lookup("Write", ""); ok {
		t.Error("an empty resolution must match nothing")
	}
}
