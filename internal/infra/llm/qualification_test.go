package llm

// The prompt-cache qualification suite from docs/design/prompt-cache-control.md
// section 9, phase 4.
//
// Everything else in this package tests BuildMax against a fake upstream, which
// proves what BuildMax sends and nothing about what a provider does with it. A
// cache is the one feature where that gap matters: the request can be perfectly
// shaped and the provider can still decline to cache it, for a minimum prefix
// length, an unsupported model, a platform that never implemented the field, or
// a retention window that expired. None of that is visible from a fixture.
//
// So this suite calls a real, paid provider, and it is not part of any check. It
// skips unless BUILDMAX_CACHE_QUALIFY_* names one, exactly like the store
// integration tests skip without a DSN. Run it with `./make cache-qualify`.
//
// It is the gate the design puts in front of describing a target as
// cache-capable: a provider is not listed as supported until its request shape
// and its usage response both survive these scenarios.

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// qualifyPrefixTokens is roughly how large the stable prefix is made.
//
// Providers impose a model-specific minimum below which they cache nothing, and
// a suite that used a short prompt would report "no cache" for every target and
// look like a BuildMax bug. This is comfortably above the largest minimum any
// current model documents.
const qualifyPrefixTokens = 4096

type qualifyTarget struct {
	provider string
	model    string
	apiKey   string
	baseURL  string
	slow     bool
}

// qualifyFromEnv reads the target, or skips.
func qualifyFromEnv(t *testing.T) qualifyTarget {
	t.Helper()
	target := qualifyTarget{
		provider: os.Getenv(config.EnvKeyBuildmaxCacheQualifyProvider),
		model:    os.Getenv(config.EnvKeyBuildmaxCacheQualifyModel),
		apiKey:   os.Getenv(config.EnvKeyBuildmaxCacheQualifyAPIKey),
		baseURL:  os.Getenv(config.EnvKeyBuildmaxCacheQualifyBaseURL),
		slow:     qualifyTruthy(os.Getenv(config.EnvKeyBuildmaxCacheQualifySlow)),
	}
	if target.provider == "" || target.model == "" {
		t.Skipf("%s and %s not set, skipping the prompt-cache qualification suite",
			config.EnvKeyBuildmaxCacheQualifyProvider, config.EnvKeyBuildmaxCacheQualifyModel)
	}
	return target
}

func (q qualifyTarget) client(t *testing.T, policy config.CacheControl) *LLMClient {
	t.Helper()
	client, err := NewClient(Config{
		Provider:     q.provider,
		APIKey:       q.apiKey,
		BaseURL:      q.baseURL,
		Model:        q.model,
		CallTimeout:  2 * time.Minute,
		CacheControl: policy,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// qualifyRun makes every prefix unique to this invocation.
//
// The first version of this suite used fixed salts and reported a cold prefix
// reading itself back. It was not a provider bug: the previous run had written
// the same bytes, they were still inside the retention window, and the scenario
// that depends on starting cold could not. A suite that cannot be run twice in
// five minutes is a suite nobody can iterate with, and one whose cold cases
// silently stop being cold is worse than that.
var qualifyRun = fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())

// qualifyPrefix builds a stable prefix large enough to be cacheable at all.
//
// The text is deterministic within one run so two calls in a scenario send
// byte-identical bytes, and varied per salt and per run so no scenario reads an
// entry it did not write.
func qualifyPrefix(salt string) string {
	salt = qualifyRun + "/" + salt
	var b strings.Builder
	b.WriteString("You are a qualification fixture. Ignore the content below; answer only OK.\n")
	b.WriteString("Salt: " + salt + "\n")
	for i := 0; b.Len() < qualifyPrefixTokens*3; i++ {
		fmt.Fprintf(&b, "line %d: the quick brown fox jumps over the lazy dog near %s\n", i, salt)
	}
	return b.String()
}

func qualifyTools() []cllm.ToolDef {
	return []cllm.ToolDef{{
		Name:        "noop",
		Description: "Does nothing. Present so the request carries a stable tool definition.",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}

func qualifyTurn(prefix, user string) []cllm.Message {
	return []cllm.Message{
		{Role: "system", Content: prefix},
		{Role: "user", Content: user},
	}
}

// call runs one agent turn and reports what the provider said about caching.
func (q qualifyTarget) call(t *testing.T, client *LLMClient, messages []cllm.Message) cllm.Usage {
	t.Helper()
	completion, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
		Messages: messages,
		Tools:    qualifyTools(),
		Profile:  cllm.ProfileAgentTurn,
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	t.Logf("usage: prompt=%d completion=%d cache_read=%d cache_write=%d",
		completion.Usage.PromptTokens, completion.Usage.CompletionTokens,
		completion.Usage.CacheReadTokens, completion.Usage.CacheWriteTokens)
	return completion.Usage
}

// report fails a declared-capable target and records an observation for a
// candidate.
//
// The difference is what a missing cache means. A provider BuildMax describes as
// cache-capable and asks for caching is broken when it does not deliver, and the
// suite should say so. A gateway BuildMax sends nothing to and that caches
// nothing is behaving exactly as documented — it simply does not earn a profile,
// which is a finding rather than a failure.
func (q qualifyTarget) report(t *testing.T, capability cacheCapability, msg string) {
	t.Helper()
	if capability.requestControls {
		t.Error(msg)
		return
	}
	t.Logf("NOT QUALIFIED: %s", msg)
}

// TestCacheQualification is the whole suite. Subtests are the scenarios the
// design names; each one reports what the provider actually did.
func TestCacheQualification(t *testing.T) {
	target := qualifyFromEnv(t)
	t.Logf("qualifying provider=%q model=%q", target.provider, target.model)

	capability := cacheCapabilityFor(target.provider)
	// A target that takes no cache instructions is not one with nothing to
	// qualify — it is the candidate case. This is how an OpenAI-compatible
	// gateway earns a profile: BuildMax sends it nothing, and the run answers
	// whether it caches on its own and whether it says so in a shape the
	// counters can read. Both halves of the acceptance criterion are the
	// request shape and the usage response, and the second half applies to a
	// target BuildMax never asked anything of.
	//
	// Only the scenarios that require sending a control are skipped there.
	if !capability.requestControls {
		t.Logf("provider %q takes no cache instructions; observing what it does unasked. "+
			"A pass here qualifies its usage reporting, not a request shape.", target.provider)
	}

	// A cold call means different things to the two classes, and one assertion
	// for both was wrong: an implicitly-caching gateway reports nothing on a
	// cold prefix and is behaving correctly, because it never charges for a
	// write. Only a target BuildMax asked to cache owes a write here.
	t.Run("a cold prefix", func(t *testing.T) {
		client := target.client(t, config.CacheControl{Mode: config.CacheModeAuto})
		prefix := qualifyPrefix("first-write")
		usage := target.call(t, client, qualifyTurn(prefix, "say OK"))
		if usage.CacheReadTokens > 0 {
			t.Errorf("a cold prefix read %d cached tokens; the entry is keyed more loosely than "+
				"the prompt, which would serve one prompt's cache to another", usage.CacheReadTokens)
		}
		if !capability.requestControls {
			t.Logf("no write reported, which is expected of a gateway that caches implicitly: " +
				"it charges for reads and not for writes")
			return
		}
		if usage.CacheWriteTokens == 0 {
			t.Errorf("BuildMax asked this target to cache and it wrote nothing; the prefix may be "+
				"below this model's minimum of %d tokens, or the model may not cache at all",
				qualifyPrefixTokens)
		}
	})

	t.Run("second call reads", func(t *testing.T) {
		client := target.client(t, config.CacheControl{Mode: config.CacheModeAuto})
		prefix := qualifyPrefix("sequential-hit")
		target.call(t, client, qualifyTurn(prefix, "say OK"))
		// The same static prefix, one more turn of conversation behind it.
		second := target.call(t, client, append(qualifyTurn(prefix, "say OK"),
			cllm.Message{Role: "assistant", Content: "OK"},
			cllm.Message{Role: "user", Content: "say OK again"}))
		if second.CacheReadTokens == 0 {
			target.report(t, capability, "the second call over an unchanged prefix read nothing "+
				"back; a write that nothing recovers costs more than not caching")
		}
	})

	t.Run("a changed prefix does not read", func(t *testing.T) {
		client := target.client(t, config.CacheControl{Mode: config.CacheModeAuto})
		target.call(t, client, qualifyTurn(qualifyPrefix("changed-a"), "say OK"))
		second := target.call(t, client, qualifyTurn(qualifyPrefix("changed-b"), "say OK"))
		if second.CacheReadTokens > 0 {
			t.Errorf("a different prefix read %d cached tokens; the entry is keyed more loosely "+
				"than the prompt, which would serve one prompt's cache to another",
				second.CacheReadTokens)
		}
	})

	t.Run("a long history still finds the prefix", func(t *testing.T) {
		client := target.client(t, config.CacheControl{Mode: config.CacheModeAuto})
		prefix := qualifyPrefix("lookback")
		messages := qualifyTurn(prefix, "say OK")
		target.call(t, client, messages)
		// Grow the conversation well past the rolling breakpoint. The static
		// entry has to survive that: an agent run is long, and a cache that
		// only works on turn two is not worth its write.
		for i := range 8 {
			messages = append(messages,
				cllm.Message{Role: "assistant", Content: "OK"},
				cllm.Message{Role: "user", Content: fmt.Sprintf("turn %d: say OK", i)})
		}
		last := target.call(t, client, messages)
		if last.CacheReadTokens == 0 {
			target.report(t, capability, "a grown conversation stopped reading the static prefix; "+
				"lookback does not reach it and every long turn pays full price")
		}
	})

	t.Run("streaming reports the same counts", func(t *testing.T) {
		client := target.client(t, config.CacheControl{Mode: config.CacheModeAuto})
		prefix := qualifyPrefix("streaming")
		target.call(t, client, qualifyTurn(prefix, "say OK"))

		completion, err := client.ChatCompletionStreaming(context.Background(), cllm.Request{
			Messages: qualifyTurn(prefix, "say OK"),
			Tools:    qualifyTools(),
			Profile:  cllm.ProfileAgentTurn,
		}, func(string) {})
		if err != nil {
			t.Fatalf("streaming call: %v", err)
		}
		t.Logf("streamed usage: prompt=%d cache_read=%d cache_write=%d",
			completion.Usage.PromptTokens, completion.Usage.CacheReadTokens,
			completion.Usage.CacheWriteTokens)
		if completion.Usage.CacheReadTokens == 0 {
			target.report(t, capability, "a streamed call reported no cache read over a warm "+
				"prefix; usage from the event stream is where these counts get lost")
		}
	})

	t.Run("concurrent cold starts", func(t *testing.T) {
		client := target.client(t, config.CacheControl{Mode: config.CacheModeAuto})
		prefix := qualifyPrefix("concurrent")
		// Two calls racing on a cold entry. Neither is required to read — that
		// is the race — but both must come back with coherent counts rather
		// than an error or a cached count larger than the prompt.
		var wg sync.WaitGroup
		usages := make([]cllm.Usage, 2)
		for i := range usages {
			wg.Add(1)
			go func() {
				defer wg.Done()
				completion, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
					Messages: qualifyTurn(prefix, "say OK"),
					Tools:    qualifyTools(),
					Profile:  cllm.ProfileAgentTurn,
				})
				if err != nil {
					t.Errorf("concurrent call %d: %v", i, err)
					return
				}
				usages[i] = completion.Usage
			}()
		}
		wg.Wait()
		for i, usage := range usages {
			t.Logf("concurrent %d: prompt=%d cache_read=%d cache_write=%d",
				i, usage.PromptTokens, usage.CacheReadTokens, usage.CacheWriteTokens)
			if usage.CacheReadTokens+usage.CacheWriteTokens > usage.PromptTokens {
				t.Errorf("concurrent call %d reported more cached tokens than prompt tokens: %+v",
					i, usage)
			}
		}
	})

	t.Run("an explicit retention is accepted", func(t *testing.T) {
		if !capability.requestControls {
			t.Skipf("provider %q takes no cache instructions, so there is no retention to send",
				target.provider)
		}
		if len(capability.ttls) == 0 {
			t.Skipf("provider %q documents no selectable retention", target.provider)
		}
		for _, ttl := range capability.ttls {
			t.Run(ttl, func(t *testing.T) {
				client := target.client(t, config.CacheControl{Mode: config.CacheModeAuto, TTL: ttl})
				// The provider accepting the field is what is under test here.
				// Whether it honours the duration is the expiry scenario below.
				target.call(t, client, qualifyTurn(qualifyPrefix("ttl-"+ttl), "say OK"))
			})
		}
	})

	t.Run("retention expires", func(t *testing.T) {
		if !capability.requestControls {
			t.Skipf("provider %q takes no cache instructions, so no window was asked for",
				target.provider)
		}
		if !target.slow {
			t.Skipf("set %s to include the scenarios that wait out a retention window",
				config.EnvKeyBuildmaxCacheQualifySlow)
		}
		if !slices.Contains(capability.ttls, config.CacheTTL5m) {
			t.Skipf("provider %q documents no window short enough to wait out", target.provider)
		}
		client := target.client(t, config.CacheControl{Mode: config.CacheModeAuto, TTL: config.CacheTTL5m})
		prefix := qualifyPrefix("expiry")
		target.call(t, client, qualifyTurn(prefix, "say OK"))
		// Past the window, plus a margin: a provider is entitled to keep an
		// entry a little longer, and a suite that failed on that would report a
		// bug that is not one.
		t.Log("waiting out a 5m retention window")
		time.Sleep(6 * time.Minute)
		after := target.call(t, client, qualifyTurn(prefix, "say OK"))
		if after.CacheReadTokens > 0 {
			t.Logf("the entry survived a 5m window (%d tokens read); retention is at least "+
				"as long as asked for, which is not a failure", after.CacheReadTokens)
		}
		if after.CacheWriteTokens == 0 && after.CacheReadTokens == 0 {
			t.Error("after expiry the call neither read nor rewrote; a run that outlives " +
				"one window would go on paying full price with no entry to show for it")
		}
	})
}

// qualifyTruthy matches the spelling every other BUILDMAX_ flag accepts.
func qualifyTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
