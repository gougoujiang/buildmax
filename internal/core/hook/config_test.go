package hook

import "testing"

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig([]byte("post_tool_use:\n  - type: command\n    command: gofmt -w .\n"))
	if err != nil {
		t.Fatal(err)
	}
	entries := cfg.Entries(EventPostToolUse)
	if len(entries) != 1 || entries[0].Command != "gofmt -w ." {
		t.Fatalf("got %+v", entries)
	}
	if cfg.IsEmpty() {
		t.Error("a config with an entry is not empty")
	}
	if _, err := ParseConfig([]byte("post_tool_use: [ unclosed\n")); err == nil {
		t.Error("malformed YAML should be an error")
	}
	empty, err := ParseConfig(nil)
	if err != nil || !empty.IsEmpty() {
		t.Errorf("empty document = %+v, %v", empty, err)
	}
}

func TestResolvedTypeDefaultsToCommand(t *testing.T) {
	if got := (Entry{}).ResolvedType(); got != TypeCommand {
		t.Errorf("ResolvedType() = %q, want %q", got, TypeCommand)
	}
	if got := (Entry{Type: TypeHTTP}).ResolvedType(); got != TypeHTTP {
		t.Errorf("ResolvedType() = %q, want %q", got, TypeHTTP)
	}
}

// EachEntry and Entries must cover the same events. An event added to
// the struct but missed in either walk would silently skip expansion and
// inspection, which is exactly the failure neither would report.
func TestEachEntryVisitsEveryEvent(t *testing.T) {
	var doc string
	for _, event := range EventNames() {
		doc += yamlKeyFor(t, event) + ":\n  - type: command\n    command: " + event + "\n"
	}
	cfg, err := ParseConfig([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	count := 0
	cfg.EachEntry(func(e *Entry) {
		seen[e.Command] = true
		count++
	})
	if count != len(EventNames()) {
		t.Fatalf("visited %d entries, want %d — an event is missing from the walk", count, len(EventNames()))
	}
	for _, event := range EventNames() {
		if !seen[event] {
			t.Errorf("EachEntry skipped %s", event)
		}
		if len(cfg.Entries(event)) != 1 {
			t.Errorf("Entries(%s) did not decode", event)
		}
	}
	if cfg.Entries("NoSuchEvent") != nil {
		t.Error("an unknown event should return nothing")
	}
}

// EachEntry hands out pointers so a caller can rewrite entries in place, which
// is how plugin path expansion works.
func TestEachEntryEditsInPlace(t *testing.T) {
	cfg, err := ParseConfig([]byte("pre_tool_use:\n  - type: command\n    command: original\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.EachEntry(func(e *Entry) { e.Command = "rewritten" })
	if got := cfg.Entries(EventPreToolUse)[0].Command; got != "rewritten" {
		t.Errorf("Command = %q, want the edit to stick", got)
	}
}

// yamlKeyFor maps a canonical event name to the snake_case document key.
func yamlKeyFor(t *testing.T, event string) string {
	t.Helper()
	keys := map[string]string{
		EventSessionStart: "session_start", EventSessionEnd: "session_end",
		EventUserPromptSubmit: "user_prompt_submit", EventPreToolUse: "pre_tool_use",
		EventPostToolUse: "post_tool_use", EventPostToolUseFailure: "post_tool_use_failure",
		EventNotification: "notification", EventPreCompact: "pre_compact",
		EventPostCompact: "post_compact", EventSubagentStart: "subagent_start",
		EventSubagentStop: "subagent_stop", EventStop: "stop", EventStopFailure: "stop_failure",
		EventWorktreeCreate: "worktree_create", EventWorktreeRemove: "worktree_remove",
		EventCwdChanged: "cwd_changed",
	}
	key, ok := keys[event]
	if !ok {
		t.Fatalf("event %q has no document key in this test; add it", event)
	}
	return key
}
