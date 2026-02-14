// Package util provides shared helpers reusable across the project.
//
// argparse.go — argument-parsing helpers for JSON-decoded map[string]any args.
// JSON numbers arrive as float64; int and int64 are also handled.
package util

import (
	"fmt"
	"strings"
)

// ParseRequiredString extracts a required string argument from args, trims it,
// and returns an error if the key is missing, not a string, or empty after trimming.
func ParseRequiredString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%s is empty", key)
	}
	return s, nil
}

// ParseOptionalString extracts an optional string argument with a default value.
// Returns defaultVal if the key is missing, nil, not a string, or empty after trimming.
func ParseOptionalString(args map[string]any, key, defaultVal string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	return s
}

// ParseOptionalBool extracts an optional boolean argument with a default value.
// Returns defaultVal if the key is missing, nil, or not a bool.
func ParseOptionalBool(args map[string]any, key string, defaultVal bool) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// ParseOptionalInt extracts an optional non-negative integer argument with a default value.
// JSON numbers arrive as float64; int and int64 are also accepted.
// Returns defaultVal if the key is missing, nil, not a number, or negative.
func ParseOptionalInt(args map[string]any, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	switch x := v.(type) {
	case float64:
		if x >= 0 {
			return int(x)
		}
	case int:
		if x >= 0 {
			return x
		}
	case int64:
		if x >= 0 {
			return int(x)
		}
	}
	return defaultVal
}

// ToFloat64 attempts to convert v to float64.
// Handles float64, int, and int64 types (common in JSON-decoded values).
func ToFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	default:
		return 0, false
	}
}
