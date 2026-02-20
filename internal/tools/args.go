// Package tools provides concrete agent tools and shared argument helpers.
package tools

import (
	"errors"
	"strings"

	"buildmax/internal/util"
)

// RequiredString returns the string value for key or an error if missing or not a string.
// The value is trimmed of surrounding whitespace; empty after trim returns error.
func RequiredString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", errors.New("missing " + key)
	}
	s, ok := v.(string)
	if !ok {
		return "", errors.New(key + " must be a string")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New(key + " is empty")
	}
	return s, nil
}

// OptionalString returns the string value for key and true if present and non-nil, or ("", false) otherwise.
// The value is trimmed; empty string after trim is still returned as ("", true).
func OptionalString(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(s), true
}

// OptionalInt returns the integer value for key (JSON numbers come as float64), or defaultVal if missing or not a number.
func OptionalInt(args map[string]any, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	f, ok := util.ToFloat64(v)
	if !ok {
		return defaultVal
	}
	return int(f)
}
