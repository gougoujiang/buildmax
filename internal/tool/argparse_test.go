package tool

import (
	"math"
	"testing"
)

func TestParseRequiredString(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		want    string
		wantErr string
	}{
		{name: "trims value", args: map[string]any{"name": "  value\n"}, want: "value"},
		{name: "missing", args: map[string]any{}, wantErr: "missing name"},
		{name: "wrong type", args: map[string]any{"name": 1}, wantErr: "name must be a string"},
		{name: "empty after trimming", args: map[string]any{"name": " \t"}, wantErr: "name is empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRequiredString(tt.args, "name")
			assertParsedString(t, got, err, tt.want, tt.wantErr)
		})
	}
}

func TestParseRequiredStringRaw(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		want    string
		wantErr string
	}{
		{name: "preserves whitespace", args: map[string]any{"content": "  value\n"}, want: "  value\n"},
		{name: "accepts whitespace only", args: map[string]any{"content": " \t"}, want: " \t"},
		{name: "missing", args: map[string]any{}, wantErr: "missing content"},
		{name: "wrong type", args: map[string]any{"content": false}, wantErr: "content must be a string"},
		{name: "empty", args: map[string]any{"content": ""}, wantErr: "content is empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRequiredStringRaw(tt.args, "content")
			assertParsedString(t, got, err, tt.want, tt.wantErr)
		})
	}
}

func assertParsedString(t *testing.T, got string, err error, want, wantErr string) {
	t.Helper()
	if wantErr != "" {
		if err == nil || err.Error() != wantErr {
			t.Fatalf("error = %v, want %q", err, wantErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("value = %q, want %q", got, want)
	}
}

func TestParseOptionalString(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "trims value", args: map[string]any{"name": "  value\n"}, want: "value"},
		{name: "missing", args: map[string]any{}, want: "default"},
		{name: "nil", args: map[string]any{"name": nil}, want: "default"},
		{name: "wrong type", args: map[string]any{"name": 1}, want: "default"},
		{name: "empty after trimming", args: map[string]any{"name": " \t"}, want: "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseOptionalString(tt.args, "name", "default"); got != tt.want {
				t.Errorf("value = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseOptionalBool(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want bool
	}{
		{name: "true", args: map[string]any{"enabled": true}, want: true},
		{name: "false overrides true default", args: map[string]any{"enabled": false}, want: false},
		{name: "missing", args: map[string]any{}, want: true},
		{name: "nil", args: map[string]any{"enabled": nil}, want: true},
		{name: "wrong type", args: map[string]any{"enabled": "false"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseOptionalBool(tt.args, "enabled", true); got != tt.want {
				t.Errorf("value = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestParseOptionalInt(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want int
	}{
		{name: "float64", args: map[string]any{"limit": float64(12)}, want: 12},
		{name: "int", args: map[string]any{"limit": 13}, want: 13},
		{name: "int64", args: map[string]any{"limit": int64(14)}, want: 14},
		{name: "zero", args: map[string]any{"limit": 0}, want: 0},
		{name: "missing", args: map[string]any{}, want: 7},
		{name: "nil", args: map[string]any{"limit": nil}, want: 7},
		{name: "wrong type", args: map[string]any{"limit": "12"}, want: 7},
		{name: "negative float64", args: map[string]any{"limit": float64(-1)}, want: 7},
		{name: "negative int", args: map[string]any{"limit": -1}, want: 7},
		{name: "negative int64", args: map[string]any{"limit": int64(-1)}, want: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseOptionalInt(tt.args, "limit", 7); got != tt.want {
				t.Errorf("value = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		want   float64
		wantOK bool
	}{
		{name: "float64", value: 1.25, want: 1.25, wantOK: true},
		{name: "int", value: 2, want: 2, wantOK: true},
		{name: "int64", value: int64(-3), want: -3, wantOK: true},
		{name: "nan", value: math.NaN(), want: math.NaN(), wantOK: true},
		{name: "wrong type", value: "1.25", want: 0, wantOK: false},
		{name: "nil", value: nil, want: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toFloat64(tt.value)
			if ok != tt.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tt.wantOK)
			}
			if math.IsNaN(tt.want) {
				if !math.IsNaN(got) {
					t.Errorf("value = %v, want NaN", got)
				}
			} else if got != tt.want {
				t.Errorf("value = %v, want %v", got, tt.want)
			}
		})
	}
}
