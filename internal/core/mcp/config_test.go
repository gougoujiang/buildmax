package mcp

import "testing"

func TestParseConfig(t *testing.T) {
	root, err := ParseConfig([]byte(`{"mcpServers":{"a":{"type":"stdio","command":"x"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if root == nil || root.MCPServers["a"].Command != "x" {
		t.Fatalf("got %+v", root)
	}

	// An empty document and a missing file mean the same thing to every caller.
	for _, src := range []string{`{}`, `{"mcpServers":{}}`} {
		root, err := ParseConfig([]byte(src))
		if err != nil || root != nil {
			t.Errorf("ParseConfig(%s) = %+v, %v; want nil, nil", src, root, err)
		}
	}
	if _, err := ParseConfig([]byte("{not json")); err == nil {
		t.Error("malformed JSON should be an error")
	}
}

func TestValidateServerConfig(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		cfg     ServerConfig
		wantErr bool
	}{
		{"stdio with command", "a", ServerConfig{Type: TransportStdio, Command: "x"}, false},
		{"stdio without command", "a", ServerConfig{Type: TransportStdio}, true},
		{"http with url", "a", ServerConfig{Type: TransportHTTP, URL: "https://x"}, false},
		{"sse with url", "a", ServerConfig{Type: TransportSSE, URL: "https://x"}, false},
		{"http without url", "a", ServerConfig{Type: TransportHTTP}, true},
		{"unknown transport", "a", ServerConfig{Type: "carrier-pigeon", Command: "x"}, true},
		{"missing transport", "a", ServerConfig{Command: "x"}, true},
		{"empty id", "", ServerConfig{Type: TransportStdio, Command: "x"}, true},
		// Validation runs against the document as written, so an unexpanded
		// variable is a present command, not a missing one.
		{"unexpanded command", "a", ServerConfig{Type: TransportStdio, Command: "${BUILDMAX_PLUGIN_ROOT}/bin/x"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateServerConfig(tc.id, tc.cfg)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateServerConfig = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
