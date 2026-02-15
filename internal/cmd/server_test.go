package cmd

import (
	"os"
	"strconv"
	"testing"
)

func TestResolveServerPort(t *testing.T) {
	tests := []struct {
		name     string
		flagPort int
		env      string // BUILDMAX_SERVER_PORT, empty means unset
		wantPort int
		wantErr  bool
	}{
		{"flag overrides env", 9999, "8888", 9999, false},
		{"env when flag zero", 0, "8888", 8888, false},
		{"default when no flag and no env", 0, "", defaultServerPort, false},
		{"invalid env", 0, "bad", 0, true},
		{"env zero is invalid", 0, "0", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				os.Setenv("BUILDMAX_SERVER_PORT", tt.env)
				defer os.Unsetenv("BUILDMAX_SERVER_PORT")
			} else {
				os.Unsetenv("BUILDMAX_SERVER_PORT")
			}

			c := newServerCommand()
			if tt.flagPort > 0 {
				_ = c.Flags().Set("port", strconv.Itoa(tt.flagPort))
			}
			port, err := resolveServerPort(c)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveServerPort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && port != tt.wantPort {
				t.Errorf("resolveServerPort() = %d, want %d", port, tt.wantPort)
			}
		})
	}
}
