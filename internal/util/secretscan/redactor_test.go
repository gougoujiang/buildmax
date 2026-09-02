package secretscan

import "testing"

func TestRedactor_ExactValues(t *testing.T) {
	r := NewRedactor([]string{"ghs_abcdef123456", "short", "", "AKIAIOSFODNN7EXAMPLE"})
	// A registered value is replaced wherever it appears.
	if got := r.Redact("token is ghs_abcdef123456 in the log"); got != "token is [redacted] in the log" {
		t.Fatalf("exact redaction: got %q", got)
	}
	// A value below the length floor is not registered, so it is not replaced
	// just for being in the list.
	if got := r.Redact("this is short here"); got != "this is short here" {
		t.Fatalf("short value should not be redacted: got %q", got)
	}
	// Shape-based redaction still runs on top of exact.
	if got := r.Redact("Authorization: Bearer sometoken12345"); got == "Authorization: Bearer sometoken12345" {
		t.Fatalf("shape redaction should still apply: got %q", got)
	}
}

func TestRedactor_NilRedactsByShapeOnly(t *testing.T) {
	var r *Redactor
	if got := r.Redact("sk-abcdefghijklmnop123"); got == "sk-abcdefghijklmnop123" {
		t.Fatalf("nil redactor should still redact shapes: got %q", got)
	}
}
