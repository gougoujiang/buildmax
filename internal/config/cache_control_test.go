package config

import "testing"

func boolPtr(v bool) *bool { return &v }

// The migration in docs/design/prompt-cache-control.md section 4.
//
// The distinction the pointer exists for: absent means nobody chose, and false
// means someone chose no. Collapsing them would either take caching away from
// every model whose owner never thought about it, or turn an explicit opt-out
// back on.
func TestResolveCacheControl(t *testing.T) {
	tests := []struct {
		name       string
		structured *CacheControl
		legacy     *bool
		wantMode   string
		wantTTL    string
	}{
		{
			name:     "nothing configured takes the default",
			wantMode: CacheModeAuto, wantTTL: CacheTTLProviderDefault,
		},
		{
			// True was written when the only alternative was no caching at all,
			// so the weaker of the two new modes would take away what it asked
			// for.
			name:   "the legacy true becomes force",
			legacy: boolPtr(true), wantMode: CacheModeForce, wantTTL: CacheTTLProviderDefault,
		},
		{
			name:   "an explicit legacy false stays an opt-out",
			legacy: boolPtr(false), wantMode: CacheModeOff, wantTTL: CacheTTLProviderDefault,
		},
		{
			name:       "the structured policy wins over the shorthand",
			structured: &CacheControl{Mode: CacheModeOff},
			legacy:     boolPtr(true),
			wantMode:   CacheModeOff, wantTTL: CacheTTLProviderDefault,
		},
		{
			name:       "a partial structured policy fills its own defaults",
			structured: &CacheControl{TTL: CacheTTL1h},
			wantMode:   CacheModeAuto, wantTTL: CacheTTL1h,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveCacheControl(tc.structured, tc.legacy)
			if got.Mode != tc.wantMode || got.TTL != tc.wantTTL {
				t.Errorf("got %+v, want mode %q ttl %q", got, tc.wantMode, tc.wantTTL)
			}
		})
	}
}

// A typo names itself at load rather than arriving as a provider error, or
// worse, as silence.
func TestValidateCacheControl(t *testing.T) {
	for _, ok := range []CacheControl{
		{},
		{Mode: CacheModeAuto, TTL: CacheTTLProviderDefault},
		{Mode: CacheModeForce, TTL: CacheTTL1h},
		{Mode: CacheModeOff},
	} {
		if err := ValidateCacheControl(ok); err != nil {
			t.Errorf("ValidateCacheControl(%+v) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []CacheControl{
		{Mode: "on"},
		{Mode: "AUTO"},
		{TTL: "10m"},
		{Mode: CacheModeAuto, TTL: "forever"},
	} {
		if err := ValidateCacheControl(bad); err == nil {
			t.Errorf("ValidateCacheControl(%+v) = nil, want an error", bad)
		}
	}
}
