package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

func toolset(names ...string) []cllm.ToolDef {
	out := make([]cllm.ToolDef, 0, len(names))
	for _, n := range names {
		out = append(out, cllm.ToolDef{
			Name:        n,
			Description: "does " + n,
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
		})
	}
	return out
}

// A key has to be the same across the calls that should share a bucket, or
// nothing ever hits.
func TestCacheKeyIsStableForTheSameStaticInput(t *testing.T) {
	first := deriveCacheKey("sk-abc", "gpt-5", "tm_one", "be brief", toolset("read", "write"))
	second := deriveCacheKey("sk-abc", "gpt-5", "tm_one", "be brief", toolset("read", "write"))
	if first != second {
		t.Errorf("two identical calls derived different keys: %q and %q", first, second)
	}
	if !strings.HasPrefix(first, cacheKeyVersion+"-") {
		t.Errorf("key %q does not carry a derivation version", first)
	}
}

// The acceptance criterion from docs/design/prompt-cache-control.md section 9,
// phase 3: keys change across team and static-prefix boundaries.
//
// Each input here is one that has to match for the provider to hit. A key that
// survived a change to any of them would ask the provider to look up a prefix
// that is no longer being sent, and every call would miss while looking fine.
func TestCacheKeySeparatesEveryBucketBoundary(t *testing.T) {
	base := func() string {
		return deriveCacheKey("sk-abc", "gpt-5", "tm_one", "be brief", toolset("read", "write"))
	}
	tests := map[string]string{
		"a different credential": deriveCacheKey("sk-xyz", "gpt-5", "tm_one", "be brief", toolset("read", "write")),
		"a different model":      deriveCacheKey("sk-abc", "gpt-4", "tm_one", "be brief", toolset("read", "write")),
		"a different team":       deriveCacheKey("sk-abc", "gpt-5", "tm_two", "be brief", toolset("read", "write")),
		"a changed system prompt": deriveCacheKey("sk-abc", "gpt-5", "tm_one", "be thorough",
			toolset("read", "write")),
		"a tool added":   deriveCacheKey("sk-abc", "gpt-5", "tm_one", "be brief", toolset("read", "write", "bash")),
		"a tool removed": deriveCacheKey("sk-abc", "gpt-5", "tm_one", "be brief", toolset("read")),
		// Order is part of the prefix: the same tools in another order render
		// to different bytes and cache separately, so claiming a match here
		// would claim one the provider will not make.
		"tools reordered": deriveCacheKey("sk-abc", "gpt-5", "tm_one", "be brief", toolset("write", "read")),
	}
	for name, other := range tests {
		t.Run(name, func(t *testing.T) {
			if other == base() {
				t.Errorf("%s produced the same bucket", name)
			}
		})
	}
}

// The key must not leak what it was derived from. The credential is the case
// that matters: it separates two accounts and must not appear in a field that
// travels to a provider and shows up in its logs.
func TestCacheKeyCarriesNoInputVerbatim(t *testing.T) {
	key := deriveCacheKey("sk-secret-credential", "gpt-5", "tm_one",
		"the workspace is /home/alice/project", toolset("read"))
	for _, secret := range []string{
		"sk-secret-credential", "secret", "tm_one", "alice", "/home/alice", "workspace", "read",
	} {
		if strings.Contains(key, secret) {
			t.Errorf("key %q contains %q", key, secret)
		}
	}
}

// Field boundaries are length-delimited, so two different splits of the same
// bytes cannot collide into one bucket.
func TestCacheKeyDoesNotCollideAcrossFieldBoundaries(t *testing.T) {
	first := deriveCacheKey("sk", "abc", "", "", nil)
	second := deriveCacheKey("sk", "ab", "c", "", nil)
	if first == second {
		t.Error("two different field splits produced one key")
	}
}

// The request-shape half of the phase 3 acceptance: the key and the retention
// reach the wire, and only on a call the policy accepted.
func TestResponsesSendsAScopedCacheKey(t *testing.T) {
	tests := []struct {
		name    string
		ttl     string
		profile cllm.CallProfile
		wantKey bool
		wantTTL string
	}{
		{name: "an agent turn is scoped", profile: cllm.ProfileAgentTurn, wantKey: true},
		{name: "a title is not", profile: cllm.ProfileTitle},
		{
			name: "extended retention is sent when asked for",
			ttl:  config.CacheTTL24h, profile: cllm.ProfileAgentTurn, wantKey: true, wantTTL: "24h",
		},
		{
			name: "the provider default pins no retention",
			ttl:  config.CacheTTLProviderDefault, profile: cllm.ProfileAgentTurn, wantKey: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			up := newUpstreamWithBody(t, mustJSON(map[string]any{
				"id": "resp_1", "object": "response", "status": "completed", "model": "m",
				"output": []any{map[string]any{
					"type": "message", "role": "assistant",
					"content": []any{map[string]any{"type": "output_text", "text": "ok"}},
				}},
			}))
			client, err := NewClient(Config{
				Provider: cllm.ProviderOpenAI, APIKey: "sk-test", BaseURL: up.server.URL,
				Model: "gpt-5", CacheControl: config.CacheControl{Mode: config.CacheModeAuto, TTL: tc.ttl},
			})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if _, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
				Messages:   []cllm.Message{{Role: "system", Content: "be brief"}, {Role: "user", Content: "hi"}},
				Tools:      toolset("read"),
				Profile:    tc.profile,
				CacheScope: "tm_one",
			}); err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}

			var request map[string]any
			if err := json.Unmarshal([]byte(up.bodies[0]), &request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			key, hasKey := request["prompt_cache_key"].(string)
			if hasKey != tc.wantKey {
				t.Errorf("prompt_cache_key present = %v, want %v: %s", hasKey, tc.wantKey, up.bodies[0])
			}
			if tc.wantKey && !strings.HasPrefix(key, cacheKeyVersion+"-") {
				t.Errorf("key %q is not a derived one", key)
			}
			// The key is opaque on the wire: the credential and the scope that
			// produced it must not travel with it.
			if strings.Contains(up.bodies[0], "sk-test") || strings.Contains(key, "tm_one") {
				t.Errorf("request %s leaked a key input", up.bodies[0])
			}
			retention, _ := request["prompt_cache_retention"].(string)
			if retention != tc.wantTTL {
				t.Errorf("prompt_cache_retention = %q, want %q", retention, tc.wantTTL)
			}
		})
	}
}

// A key is never written down. It is an input to one request and nothing else:
// the ledger, the trace, and the session all record counts and outcomes, and a
// stored bucket identifier would be a durable handle on someone's prompt
// population that nothing needs.
func TestCacheKeyIsNotOnTheResult(t *testing.T) {
	up := newUpstreamWithBody(t, mustJSON(map[string]any{
		"id": "resp_1", "object": "response", "status": "completed", "model": "m",
		"output": []any{map[string]any{
			"type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": "ok"}},
		}},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 4, "total_tokens": 14},
	}))
	client, err := NewClient(Config{
		Provider: cllm.ProviderOpenAI, APIKey: "sk-test", BaseURL: up.server.URL, Model: "gpt-5",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	completion, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
		Messages: []cllm.Message{{Role: "user", Content: "hi"}}, Profile: cllm.ProfileAgentTurn,
	})
	if err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	encoded, err := json.Marshal(completion)
	if err != nil {
		t.Fatalf("marshal completion: %v", err)
	}
	if strings.Contains(string(encoded), cacheKeyVersion) {
		t.Errorf("the result carries a cache key: %s", encoded)
	}
}
