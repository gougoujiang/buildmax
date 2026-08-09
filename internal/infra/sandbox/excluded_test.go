package sandbox

import "testing"

func TestMatchesExcluded(t *testing.T) {
	cases := []struct {
		name    string
		command string
		list    []string
		want    bool
	}{
		{"empty list", "ls", nil, false},
		{"exact match", "make build", []string{"make build"}, true},
		{"exact mismatch", "make test", []string{"make build"}, false},
		{"prefix-colon-star matches base", "npm", []string{"npm:*"}, true},
		{"prefix-colon-star matches subcommand", "npm install -g foo", []string{"npm:*"}, true},
		{"prefix-space-star requires space", "docker ps", []string{"docker *"}, true},
		{"prefix-space-star rejects bare", "docker", []string{"docker *"}, false},
		{"compound second segment matches", "make build && docker ps", []string{"docker *"}, true},
		{"compound via pipe", "echo x | docker ps", []string{"docker *"}, true},
		{"compound via or", "false || npm test", []string{"npm:*"}, true},
		{"compound via semicolon", "ls ; docker ps", []string{"docker *"}, true},
		{"no false match on substring", "echo docker", []string{"docker *"}, false},
		{"empty prefix pattern ignored", "anything", []string{":*", " *"}, false},
		{"multiple patterns first match wins", "go test ./...", []string{"npm:*", "go test:*"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchesExcluded(tc.command, tc.list); got != tc.want {
				t.Errorf("MatchesExcluded(%q, %v) = %v, want %v", tc.command, tc.list, got, tc.want)
			}
		})
	}
}
