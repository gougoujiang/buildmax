package identity

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func TestHashPasswordRoundTrips(t *testing.T) {
	const plaintext = "correct horse battery staple"
	hash, err := HashPassword(plaintext)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !VerifyPassword(hash, plaintext) {
		t.Error("the password does not verify against its own hash")
	}
	if VerifyPassword(hash, plaintext+"x") {
		t.Error("a different password verified")
	}
}

// The stored value must not be derivable from the password alone, or a stolen
// dump would be a rainbow-table exercise rather than a per-row attack.
func TestHashPasswordIsSaltedPerCall(t *testing.T) {
	const plaintext = "correct horse battery staple"
	first, err := HashPassword(plaintext)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	second, err := HashPassword(plaintext)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if first == second {
		t.Fatal("hashing the same password twice produced the same value")
	}
	// Both still verify: the salt travels in the encoded hash.
	if !VerifyPassword(first, plaintext) || !VerifyPassword(second, plaintext) {
		t.Error("a salted hash does not verify")
	}
	if strings.Contains(first, plaintext) {
		t.Error("the encoded hash contains the plaintext")
	}
}

func TestHashPasswordEncodesItsParameters(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// The parameters ride along so that raising them later leaves existing
	// hashes verifiable instead of locking everyone out.
	for _, want := range []string{"$argon2id$", "m=", "t=", "p="} {
		if !strings.Contains(hash, want) {
			t.Errorf("hash %q does not carry %q", hash, want)
		}
	}
}

// A hash that cannot be parsed must never authenticate. This is the failure
// mode where a truncated column or a half-migrated row becomes a way in.
func TestVerifyPasswordRefusesMalformedHashes(t *testing.T) {
	valid, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	tests := []struct {
		name string
		hash string
	}{
		{"empty", ""},
		{"not a hash", "correct horse battery staple"},
		{"wrong algorithm", strings.Replace(valid, "argon2id", "argon2i", 1)},
		{"truncated", valid[:len(valid)/2]},
		{"empty key", "$argon2id$v=19$m=65536,t=1,p=4$c2FsdHNhbHRzYWx0c2E$"},
		{"unsupported version", strings.Replace(valid, "v=19", "v=18", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if VerifyPassword(tt.hash, "correct horse battery staple") {
				t.Error("a malformed hash authenticated")
			}
			// And it must not authenticate an empty password either, which is
			// what a naive "compare what we parsed" would do.
			if VerifyPassword(tt.hash, "") {
				t.Error("a malformed hash authenticated an empty password")
			}
		})
	}
}

func TestValidatePasswordEnforcesLengthOnly(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		wantError error
	}{
		{"too short", strings.Repeat("a", PasswordMinLength-1), ErrPasswordTooShort},
		{"empty", "", ErrPasswordTooShort},
		{"at the minimum", strings.Repeat("a", PasswordMinLength), nil},
		{"too long", strings.Repeat("a", PasswordMaxLength+1), ErrPasswordTooLong},
		{"at the maximum", strings.Repeat("a", PasswordMaxLength), nil},
		// No composition rule: a long passphrase of plain words is exactly what
		// the length minimum is meant to encourage.
		{"a passphrase with no digits or symbols", "correct horse battery staple", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidatePassword(tt.password); got != tt.wantError {
				t.Errorf("ValidatePassword = %v, want %v", got, tt.wantError)
			}
		})
	}
}

// Length is counted in characters, not bytes, so a short non-ASCII passphrase
// is not accepted just because it encodes to enough bytes.
func TestValidatePasswordCountsRunes(t *testing.T) {
	// Eight characters, twenty-four bytes.
	if err := ValidatePassword(strings.Repeat("密", 8)); err != ErrPasswordTooShort {
		t.Errorf("ValidatePassword = %v, want ErrPasswordTooShort", err)
	}
	if err := ValidatePassword(strings.Repeat("密", PasswordMinLength)); err != nil {
		t.Errorf("ValidatePassword = %v, want nil", err)
	}
}

func TestHashPasswordRefusesAnUnusablePassword(t *testing.T) {
	if _, err := HashPassword("short"); err != ErrPasswordTooShort {
		t.Errorf("HashPassword = %v, want ErrPasswordTooShort", err)
	}
}

func TestDummyVerifyPasswordAlwaysFails(t *testing.T) {
	if DummyVerifyPassword("anything at all") {
		t.Error("the dummy verification succeeded")
	}
}

// medianDuration runs fn n times and reports the middle result, so one
// descheduled iteration does not decide what the test saw.
func medianDuration(n int, fn func()) time.Duration {
	samples := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		fn()
		samples = append(samples, time.Since(start))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return samples[len(samples)/2]
}

// TestDummyVerifyPasswordCostsWhatAVerifyCosts is the assertion that makes
// DummyVerifyPassword worth calling.
//
// Its whole job is to take the time a real verification takes, so that a login
// for an unknown address costs the same as one for a known address with the
// wrong password. A body of `return false` satisfies every other test in this
// package -- it fails, which is all TestDummyVerifyPasswordAlwaysFails asks --
// and silently reopens the timing channel that answers "does this address have
// an account here".
//
// The threshold is loose on purpose. A real verification and this one differ by
// the salt decode and little else, so the honest ratio sits near 0.7; a stub
// would sit near zero. Anything above a quarter is the work being done.
func TestDummyVerifyPasswordCostsWhatAVerifyCosts(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	const wrong = "wrong password entirely"

	real := medianDuration(5, func() { VerifyPassword(hash, wrong) })
	dummy := medianDuration(5, func() { DummyVerifyPassword(wrong) })

	if real <= 0 {
		t.Fatalf("a real verification measured as %v; the clock is not usable here", real)
	}
	if ratio := float64(dummy) / float64(real); ratio < 0.25 {
		t.Errorf("DummyVerifyPassword cost %v against a real verification's %v (ratio %.3f).\n"+
			"It is meant to do the same work; at this ratio a login for an unknown address\n"+
			"is measurably faster than one for a known address, which answers whether the\n"+
			"address is registered.", dummy, real, ratio)
	}
}
