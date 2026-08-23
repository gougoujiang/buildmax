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

// ResolveCacheControl folds the structured policy and the deprecated
// prompt_cache shorthand into one effective policy.
//
// The shorthand is a pointer because absent and false are different requests
// and a bool cannot hold both. Absent means nobody chose, which becomes the
// default; an explicit false is an opt-out and stays one. An explicit true
// becomes force rather than auto: it was written when the only alternative was
// no caching at all, so reading it as the weaker of the two new modes would
// quietly take away what the operator asked for.
//
// The structured field wins outright when present. Someone who wrote both said
// the newer thing last.
func ResolveCacheControl(structured *CacheControl, legacy *bool) CacheControl {
	if structured != nil {
		out := *structured
		if out.Mode == "" {
			out.Mode = CacheModeAuto
		}
		if out.TTL == "" {
			out.TTL = CacheTTLProviderDefault
		}
		return out
	}
	mode := CacheModeAuto
	if legacy != nil {
		if *legacy {
			mode = CacheModeForce
		} else {
			mode = CacheModeOff
		}
	}
	return CacheControl{Mode: mode, TTL: CacheTTLProviderDefault}
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
