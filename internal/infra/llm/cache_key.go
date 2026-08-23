package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// cacheKeyVersion prefixes every derived key.
//
// It moves when the derivation changes. Without it a build that started
// including a new input would collide with one that did not, and two different
// prompt populations would share a bucket while both looked correct.
const cacheKeyVersion = "bmpc1"

// cacheKeyBytes is how much of the digest a key carries. 128 bits is far more
// than enough to keep unrelated prefixes apart, and a shorter key is one less
// long opaque string in a request nobody can read.
const cacheKeyBytes = 16

// deriveCacheKey builds the opaque bucket identifier a protocol with an explicit
// cache key sends.
//
// A provider cache key is a routing hint, not an authorization boundary: it
// decides which prefixes are looked up together, and getting that wrong is a
// correctness problem, not a security one. It still must not bucket unrelated
// populations, because a bucket shared by prompts that never match is a bucket
// that never hits.
//
// What goes in is everything that has to match for a hit to be possible: the
// credential the call authenticates with, the model, the caller's scope, and
// fingerprints of the static prefix. What stays out is everything that would
// either leak or fragment the bucket for no reason — raw prompts, messages,
// workspace paths, usernames, and the credential itself, which is hashed rather
// than carried. The result is derived per request and never persisted, logged,
// or returned.
//
// It changes when the static input changes, which favours correct bucketing
// over an optimistic hit rate: a key that outlived a changed system prompt
// would ask the provider to look up a prefix that is no longer being sent.
func deriveCacheKey(credential, model, scope string, systemPrompt string, tools []cllm.ToolDef) string {
	sum := sha256.New()
	// Length-prefixed so two different field splits cannot produce the same
	// stream — "ab"+"c" and "a"+"bc" must not be one key.
	for _, part := range []string{cacheKeyVersion, fingerprint(credential), model, scope, systemPrompt} {
		writeField(sum, part)
	}
	writeField(sum, toolsFingerprint(tools))
	return cacheKeyVersion + "-" + hex.EncodeToString(sum.Sum(nil)[:cacheKeyBytes])
}

// writeField adds one length-delimited field to a digest.
func writeField(h io.Writer, s string) {
	var length [8]byte
	n := uint64(len(s))
	for i := range length {
		length[i] = byte(n >> (8 * (7 - i)))
	}
	_, _ = h.Write(length[:])
	_, _ = io.WriteString(h, s)
}

// fingerprint hashes a value that must influence a key without appearing in it.
// The credential is the case that matters: two accounts must not share a
// bucket, and the key must not carry the secret that separates them.
func fingerprint(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// toolsFingerprint hashes the tool definitions in the order they are sent.
//
// Order is part of the prefix: the same tools in a different order render to
// different bytes and cache separately, so sorting here would claim a match the
// provider will not make.
func toolsFingerprint(tools []cllm.ToolDef) string {
	if len(tools) == 0 {
		return ""
	}
	sum := sha256.New()
	for _, t := range tools {
		writeField(sum, t.Name)
		writeField(sum, t.Description)
		// Marshalled rather than fmt-printed: a map renders in a random order
		// under %v, which would change the key between two identical requests
		// and turn every call into a miss.
		encoded, err := json.Marshal(t.Parameters)
		if err != nil {
			// A schema that cannot be encoded cannot be compared either. Its
			// name and description still separate it from other tools, and a
			// key that is stable and slightly coarse beats one that changes
			// per call.
			encoded = nil
		}
		writeField(sum, string(encoded))
	}
	return hex.EncodeToString(sum.Sum(nil))
}
