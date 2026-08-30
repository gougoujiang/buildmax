package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// loadDotEnv reads .local/env into the process environment so every task, and
// every process a task starts, sees the same values. This is the one
// implementation that replaced loadenv, loadenv.bat, and loadenv.ps1, which
// parsed the same file three different ways.
//
// Format: one KEY=value per line, an optional "export " prefix, # comments,
// blank lines, and optional surrounding quotes. Values are taken literally —
// there is no shell expansion, unlike the bash version's `source`. Entries
// override the inherited environment, matching what `set -a; source .env` did.
func loadDotEnv(root string) error {
	path := filepath.Join(root, filepath.FromSlash(localEnvPath))
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: expected KEY=value, got %q", path, i+1, strings.TrimSpace(raw))
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("%s:%d: empty key", path, i+1)
		}
		if err := os.Setenv(key, unquote(strings.TrimSpace(value))); err != nil {
			return err
		}
	}
	return nil
}

// unquote strips one layer of matching surrounding quotes, the only quoting the
// old shell loaders effectively supported.
func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	first, last := value[0], value[len(value)-1]
	if first == last && (first == '"' || first == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}
