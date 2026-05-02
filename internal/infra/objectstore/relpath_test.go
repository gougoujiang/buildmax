package objectstore

import (
	"testing"
)

func TestCleanRelPath(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty", "", "", true},
		{"dot", ".", "", true},
		{"absolute", "/a/b", "", true},
		{"traversal", "..", "", true},
		{"traversal prefix", "../x", "", true},
		{"traversal mid", "a/../b", "", true},
		{"simple", "a.txt", "a.txt", false},
		{"nested", "a/b/c.txt", "a/b/c.txt", false},
		{"backslash", "a\\b", "a/b", false},
		{"double slash", "a//b", "a/b", false},
		{"leading slash", "/a", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanRelPath(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("CleanRelPath(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("CleanRelPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
