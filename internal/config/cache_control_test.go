package config

import "testing"

// An absent block, an absent mode, and an absent ttl all mean the same thing —
// nobody chose — and all take the default. There is no shorthand to fold in:
// BuildMax is pre-release, so the bool this replaced was removed rather than
// carried alongside it.
func TestResolveCacheControl(t *testing.T) {
	tests := []struct {
		name     string
		in       *CacheControl
		wantMode string
		wantTTL  string
	}{
		{
			name:     "nothing configured takes the default",
			wantMode: CacheModeAuto, wantTTL: CacheTTLProviderDefault,
		},
		{
			name:     "an empty block is the same as none",
			in:       &CacheControl{},
			wantMode: CacheModeAuto, wantTTL: CacheTTLProviderDefault,
		},
		{
			name:     "an explicit opt-out survives",
			in:       &CacheControl{Mode: CacheModeOff},
			wantMode: CacheModeOff, wantTTL: CacheTTLProviderDefault,
		},
		{
			name:     "a partial block fills only what it left out",
			in:       &CacheControl{TTL: CacheTTL1h},
			wantMode: CacheModeAuto, wantTTL: CacheTTL1h,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveCacheControl(tc.in)
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
