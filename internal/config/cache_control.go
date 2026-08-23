package config

import (
	"fmt"
	"strings"
)

// Prompt-cache modes. A mode says what to ask a provider for; whether the ask
// is made on a given call also depends on what that call is for, which travels
// with the request as a core/llm.CallProfile.
const (
	// CacheModeAuto asks for the provider's economical default on a call whose
	// prefix will be sent again, and asks for nothing on a one-shot call. It is
	// the default.
	CacheModeAuto = "auto"
	// CacheModeOff never asks, on any call.
	CacheModeOff = "off"
	// CacheModeForce asks on every call, including ones whose prefix nothing
	// will read back. It is for a caller that knows something the runtime
	// cannot see.
	CacheModeForce = "force"
)

// Prompt-cache retention. A value other than CacheTTLProviderDefault is only
// valid where the target's protocol documents it; the client refuses the rest
// rather than sending a field the provider will ignore.
const (
	// CacheTTLProviderDefault leaves retention to the provider.
	CacheTTLProviderDefault = "provider_default"
	CacheTTL5m              = "5m"
	CacheTTL1h              = "1h"
	CacheTTL24h             = "24h"
)

// CacheControl is one model's prompt-cache policy (snake_case on disk).
type CacheControl struct {
	Mode string `mapstructure:"mode"`
	TTL  string `mapstructure:"ttl"`
}

// CacheModes and CacheTTLs are the accepted values, for validation messages.
func CacheModes() []string { return []string{CacheModeAuto, CacheModeOff, CacheModeForce} }
func CacheTTLs() []string {
	return []string{CacheTTLProviderDefault, CacheTTL5m, CacheTTL1h, CacheTTL24h}
}

// KnownCacheMode and KnownCacheTTL report whether a value names something this
// build understands. Empty is accepted as "unset" and resolves to the default.
func KnownCacheMode(mode string) bool {
	return mode == "" || mode == CacheModeAuto || mode == CacheModeOff || mode == CacheModeForce
}

func KnownCacheTTL(ttl string) bool {
	switch ttl {
	case "", CacheTTLProviderDefault, CacheTTL5m, CacheTTL1h, CacheTTL24h:
		return true
	}
	return false
}

// ResolveCacheControl fills in what a model entry left unset.
//
// An absent block, an absent mode, and an absent ttl all mean the same thing —
// nobody chose — and all take the default. There is no legacy shorthand to fold
// in: BuildMax is pre-release, so a wrong shape is corrected everywhere rather
// than carried alongside its replacement.
func ResolveCacheControl(in *CacheControl) CacheControl {
	out := CacheControl{Mode: CacheModeAuto, TTL: CacheTTLProviderDefault}
	if in == nil {
		return out
	}
	if in.Mode != "" {
		out.Mode = in.Mode
	}
	if in.TTL != "" {
		out.TTL = in.TTL
	}
	return out
}

// ValidateCacheControl reports a policy this build cannot act on. It checks the
// vocabulary only; whether a *target* supports the requested retention is the
// client's question, because it depends on the protocol the target speaks.
func ValidateCacheControl(c CacheControl) error {
	if !KnownCacheMode(c.Mode) {
		return fmt.Errorf("unknown prompt cache mode %q: use one of %s",
			c.Mode, strings.Join(CacheModes(), ", "))
	}
	if !KnownCacheTTL(c.TTL) {
		return fmt.Errorf("unknown prompt cache ttl %q: use one of %s",
			c.TTL, strings.Join(CacheTTLs(), ", "))
	}
	return nil
}
