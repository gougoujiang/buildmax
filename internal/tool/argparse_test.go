package tool

import "testing"

func TestParseRequiredString(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		key     string
		want    string
		wantErr string
	}{
		{name: "trims surrounding whitespace", args: map[string]any{"k": "  value  "}, key: "k", want: "value"},
		{name: "returns the value unchanged when already trimmed", args: map[string]any{"k": "value"}, key: "k", want: "value"},
		{name: "missing key", args: map[string]any{}, key: "k", wantErr: "missing k"},
		{name: "wrong type", args: map[string]any{"k": 42}, key: "k", wantErr: "k must be a string"},
		{name: "empty after trim", args: map[string]any{"k": "   "}, key: "k", wantErr: "k is empty"},
		{name: "empty string", args: map[string]any{"k": ""}, key: "k", wantErr: "k is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRequiredString(tt.args, tt.key)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("parseRequiredString() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRequiredString() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseRequiredString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRequiredStringRaw(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		key     string
		want    string
		wantErr string
	}{
		{name: "keeps surrounding whitespace", args: map[string]any{"k": "  value  "}, key: "k", want: "  value  "},
		{name: "missing key", args: map[string]any{}, key: "k", wantErr: "missing k"},
		{name: "wrong type", args: map[string]any{"k": true}, key: "k", wantErr: "k must be a string"},
		{name: "empty string", args: map[string]any{"k": ""}, key: "k", wantErr: "k is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRequiredStringRaw(tt.args, tt.key)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("parseRequiredStringRaw() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRequiredStringRaw() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseRequiredStringRaw() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseOptionalString(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		key        string
		defaultVal string
		want       string
	}{
		{name: "returns the value", args: map[string]any{"k": "value"}, key: "k", defaultVal: "def", want: "value"},
		{name: "trims surrounding whitespace", args: map[string]any{"k": " value "}, key: "k", defaultVal: "def", want: "value"},
		{name: "missing key", args: map[string]any{}, key: "k", defaultVal: "def", want: "def"},
		{name: "nil value", args: map[string]any{"k": nil}, key: "k", defaultVal: "def", want: "def"},
		{name: "wrong type", args: map[string]any{"k": 7}, key: "k", defaultVal: "def", want: "def"},
		{name: "empty after trim", args: map[string]any{"k": "   "}, key: "k", defaultVal: "def", want: "def"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseOptionalString(tt.args, tt.key, tt.defaultVal); got != tt.want {
				t.Fatalf("parseOptionalString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseOptionalBool(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		key        string
		defaultVal bool
		want       bool
	}{
		{name: "true", args: map[string]any{"k": true}, key: "k", defaultVal: false, want: true},
		{name: "false overrides a true default", args: map[string]any{"k": false}, key: "k", defaultVal: true, want: false},
		{name: "missing key", args: map[string]any{}, key: "k", defaultVal: true, want: true},
		{name: "nil value", args: map[string]any{"k": nil}, key: "k", defaultVal: true, want: true},
		{name: "wrong type", args: map[string]any{"k": "yes"}, key: "k", defaultVal: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseOptionalBool(tt.args, tt.key, tt.defaultVal); got != tt.want {
				t.Fatalf("parseOptionalBool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseOptionalInt(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		key        string
		defaultVal int
		want       int
	}{
		{name: "float64 value", args: map[string]any{"k": float64(5)}, key: "k", defaultVal: 1, want: 5},
		{name: "float64 zero", args: map[string]any{"k": float64(0)}, key: "k", defaultVal: 1, want: 0},
		{name: "int value", args: map[string]any{"k": 5}, key: "k", defaultVal: 1, want: 5},
		{name: "int64 value", args: map[string]any{"k": int64(5)}, key: "k", defaultVal: 1, want: 5},
		{name: "negative float64 falls back to default", args: map[string]any{"k": float64(-3)}, key: "k", defaultVal: 1, want: 1},
		{name: "negative int falls back to default", args: map[string]any{"k": -3}, key: "k", defaultVal: 1, want: 1},
		{name: "negative int64 falls back to default", args: map[string]any{"k": int64(-3)}, key: "k", defaultVal: 1, want: 1},
		{name: "missing key", args: map[string]any{}, key: "k", defaultVal: 1, want: 1},
		{name: "nil value", args: map[string]any{"k": nil}, key: "k", defaultVal: 1, want: 1},
		{name: "wrong type", args: map[string]any{"k": "5"}, key: "k", defaultVal: 1, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseOptionalInt(tt.args, tt.key, tt.defaultVal); got != tt.want {
				t.Fatalf("parseOptionalInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want float64
		ok   bool
	}{
		{name: "float64", v: 2.5, want: 2.5, ok: true},
		{name: "int", v: 3, want: 3, ok: true},
		{name: "int64", v: int64(4), want: 4, ok: true},
		{name: "string is rejected", v: "2.5", want: 0, ok: false},
		{name: "nil is rejected", v: nil, want: 0, ok: false},
		{name: "bool is rejected", v: true, want: 0, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toFloat64(tt.v)
			if ok != tt.ok {
				t.Fatalf("toFloat64() ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("toFloat64() = %v, want %v", got, tt.want)
			}
		})
	}
}
