package tool

import "testing"

func TestParseRequiredString(t *testing.T) {
	cases := []struct {
		name    string
		args    map[string]any
		want    string
		wantErr bool
	}{
		{"missing key", map[string]any{}, "", true},
		{"nil value", map[string]any{"q": nil}, "", true},
		{"wrong type int", map[string]any{"q": 42}, "", true},
		{"wrong type bool", map[string]any{"q": true}, "", true},
		{"empty string", map[string]any{"q": ""}, "", true},
		{"whitespace only", map[string]any{"q": "   \t\n "}, "", true},
		{"valid", map[string]any{"q": "hello"}, "hello", false},
		{"trims whitespace", map[string]any{"q": "  hello  "}, "hello", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRequiredString(tc.args, "q")
			if tc.wantErr && err == nil {
				t.Fatalf("parseRequiredString(%v) = %q, want error", tc.args, got)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("parseRequiredString(%v) error = %v, want %q", tc.args, err, tc.want)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("parseRequiredString(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseRequiredStringRaw(t *testing.T) {
	cases := []struct {
		name    string
		args    map[string]any
		want    string
		wantErr bool
	}{
		{"missing key", map[string]any{}, "", true},
		{"nil value", map[string]any{"q": nil}, "", true},
		{"wrong type int", map[string]any{"q": 42}, "", true},
		{"wrong type bool", map[string]any{"q": false}, "", true},
		{"empty string", map[string]any{"q": ""}, "", true},
		{"whitespace preserved", map[string]any{"q": "   "}, "   ", false},
		{"valid", map[string]any{"q": "hello"}, "hello", false},
		{"no trimming", map[string]any{"q": "  hello  "}, "  hello  ", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRequiredStringRaw(tc.args, "q")
			if tc.wantErr && err == nil {
				t.Fatalf("parseRequiredStringRaw(%v) = %q, want error", tc.args, got)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("parseRequiredStringRaw(%v) error = %v, want %q", tc.args, err, tc.want)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("parseRequiredStringRaw(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseOptionalString(t *testing.T) {
	cases := []struct {
		name       string
		args       map[string]any
		defaultVal string
		want       string
	}{
		{"missing key", map[string]any{}, "dflt", "dflt"},
		{"nil value", map[string]any{"q": nil}, "dflt", "dflt"},
		{"wrong type int", map[string]any{"q": 42}, "dflt", "dflt"},
		{"wrong type bool", map[string]any{"q": true}, "dflt", "dflt"},
		{"empty string", map[string]any{"q": ""}, "dflt", "dflt"},
		{"whitespace only", map[string]any{"q": "  \t "}, "dflt", "dflt"},
		{"valid", map[string]any{"q": "hello"}, "dflt", "hello"},
		{"trims whitespace", map[string]any{"q": "  hello  "}, "dflt", "hello"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOptionalString(tc.args, "q", tc.defaultVal); got != tc.want {
				t.Errorf("parseOptionalString(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseOptionalBool(t *testing.T) {
	cases := []struct {
		name       string
		args       map[string]any
		defaultVal bool
		want       bool
	}{
		{"missing defaults false", map[string]any{}, false, false},
		{"missing defaults true", map[string]any{}, true, true},
		{"nil defaults false", map[string]any{"flag": nil}, false, false},
		{"nil defaults true", map[string]any{"flag": nil}, true, true},
		{"wrong type string", map[string]any{"flag": "true"}, false, false},
		{"wrong type int", map[string]any{"flag": 1}, true, true},
		{"true", map[string]any{"flag": true}, false, true},
		{"false", map[string]any{"flag": false}, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOptionalBool(tc.args, "flag", tc.defaultVal); got != tc.want {
				t.Errorf("parseOptionalBool(%v, default %v) = %v, want %v", tc.args, tc.defaultVal, got, tc.want)
			}
		})
	}
}

func TestParseOptionalInt(t *testing.T) {
	cases := []struct {
		name       string
		args       map[string]any
		defaultVal int
		want       int
	}{
		{"missing key", map[string]any{}, 7, 7},
		{"nil value", map[string]any{"n": nil}, 7, 7},
		{"wrong type string", map[string]any{"n": "3"}, 7, 7},
		{"wrong type bool", map[string]any{"n": true}, 7, 7},
		{"float64 positive", map[string]any{"n": float64(3)}, 7, 3},
		{"float64 zero", map[string]any{"n": float64(0)}, 7, 0},
		{"float64 negative", map[string]any{"n": float64(-1)}, 7, 7},
		{"float64 truncates", map[string]any{"n": float64(2.7)}, 7, 2},
		{"int positive", map[string]any{"n": 5}, 7, 5},
		{"int zero", map[string]any{"n": 0}, 7, 0},
		{"int negative", map[string]any{"n": -2}, 7, 7},
		{"int64 positive", map[string]any{"n": int64(9)}, 7, 9},
		{"int64 zero", map[string]any{"n": int64(0)}, 7, 0},
		{"int64 negative", map[string]any{"n": int64(-4)}, 7, 7},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOptionalInt(tc.args, "n", tc.defaultVal); got != tc.want {
				t.Errorf("parseOptionalInt(%v, default %d) = %d, want %d", tc.args, tc.defaultVal, got, tc.want)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	cases := []struct {
		name   string
		val    any
		want   float64
		wantOK bool
	}{
		{"float64", float64(1.5), 1.5, true},
		{"float64 negative", float64(-2.5), -2.5, true},
		{"float64 zero", float64(0), 0, true},
		{"int", 3, 3, true},
		{"int negative", -4, -4, true},
		{"int64", int64(9), 9, true},
		{"int64 negative", int64(-6), -6, true},
		{"string", "1.5", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
		{"wrong type int32", int32(2), 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := toFloat64(tc.val)
			if ok != tc.wantOK {
				t.Fatalf("toFloat64(%v) ok = %v, want %v", tc.val, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("toFloat64(%v) = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}
