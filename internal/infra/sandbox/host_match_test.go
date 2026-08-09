package sandbox

import "testing"

func TestHostMatcher(t *testing.T) {
	cases := []struct {
		name    string
		allowed []string
		denied  []string
		host    string
		want    bool
	}{
		{"empty allow denies", nil, nil, "api.github.com", false},
		{"exact host matches", []string{"api.github.com"}, nil, "api.github.com", true},
		{"exact host with port", []string{"api.github.com"}, nil, "api.github.com:443", true},
		{"exact host wrong port", []string{"api.github.com:8443"}, nil, "api.github.com:443", false},
		{"exact host correct port", []string{"api.github.com:443"}, nil, "api.github.com:443", true},
		{"wildcard suffix matches", []string{"*.npmjs.org"}, nil, "registry.npmjs.org", true},
		{"wildcard suffix matches deeper", []string{"*.npmjs.org"}, nil, "a.b.npmjs.org", true},
		{"wildcard does not match bare", []string{"*.npmjs.org"}, nil, "npmjs.org", false},
		{"deny beats allow", []string{"*.example.com"}, []string{"evil.example.com"}, "evil.example.com", false},
		{"deny suffix beats specific allow", []string{"api.example.com"}, []string{"*.example.com"}, "api.example.com", false},
		{"case insensitive", []string{"API.example.com"}, nil, "api.example.com", true},
		{"trailing dot tolerated by user pattern", []string{"example.com"}, nil, "example.com", true},
		{"unknown host denied", []string{"api.github.com"}, nil, "evil.example", false},
		{"empty host denies", []string{"*"}, nil, "", false},
		{"wildcard all", []string{"*"}, nil, "any.host", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewHostMatcher(tc.allowed, tc.denied)
			got, _ := m.Allowed(tc.host)
			if got != tc.want {
				t.Errorf("Allowed(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestHostMatcher_DenyReason(t *testing.T) {
	m := NewHostMatcher(nil, []string{"evil.example"})
	ok, reason := m.Allowed("evil.example")
	if ok {
		t.Fatal("denied host reported as allowed")
	}
	if reason == "" {
		t.Error("expected non-empty deny reason")
	}
}

func TestHostMatcher_AllowAll(t *testing.T) {
	if NewHostMatcher([]string{"*"}, nil).AllowAll() != true {
		t.Error("AllowAll(*) = false")
	}
	if NewHostMatcher([]string{"example.com"}, nil).AllowAll() {
		t.Error("AllowAll for specific host returned true")
	}
}
