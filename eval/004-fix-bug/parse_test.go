package parse

import "testing"

func TestParsePositive(t *testing.T) {
	cases := []struct {
		input   string
		wantErr bool
	}{
		{"5", false},
		{"1", false},
		{"100", false},
		{"0", true},   // zero is not positive
		{"-1", true},  // negative is not positive
		{"abc", true}, // not a number
	}
	for _, c := range cases {
		_, err := ParsePositive(c.input)
		if c.wantErr && err == nil {
			t.Errorf("ParsePositive(%q) returned nil error, want error", c.input)
		}
		if !c.wantErr && err != nil {
			t.Errorf("ParsePositive(%q) returned error %v, want nil", c.input, err)
		}
	}
}
