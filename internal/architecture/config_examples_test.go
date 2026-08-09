package architecture_test

// Config-example constraints. config-examples/ is what a user copies to get
// started, so a key that exists in the config structs but appears in no example
// is a feature nobody can discover. Both gaps this catches were real: the whole
// sandbox block was missing, and worker.k8s gained config_map and home_dir
// without the example following.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

// mapstructureKeys walks t and returns every mapstructure tag, including nested
// structs, so a key added three levels down is still covered.
func mapstructureKeys(t reflect.Type, seen map[reflect.Type]bool, out map[string]bool) {
	if seen[t] {
		return
	}
	seen[t] = true
	if t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := range t.NumField() {
		f := t.Field(i)
		if tag := f.Tag.Get("mapstructure"); tag != "" {
			out[strings.Split(tag, ",")[0]] = true
		}
		mapstructureKeys(f.Type, seen, out)
	}
}

func keysOf(v any) map[string]bool {
	out := map[string]bool{}
	mapstructureKeys(reflect.TypeOf(v), map[reflect.Type]bool{}, out)
	return out
}

// assertKeysDocumented fails for every config key absent from the example file.
// Presence is enough — a commented-out sample counts, since users read examples
// to learn a key exists at all.
func assertKeysDocumented(t *testing.T, exampleFile string, keys map[string]bool, exempt map[string]bool) {
	t.Helper()
	root := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "config-examples", exampleFile))
	if err != nil {
		t.Fatalf("read %s: %v", exampleFile, err)
	}
	text := string(body)
	for key := range keys {
		if exempt[key] {
			continue
		}
		if !strings.Contains(text, key+":") {
			t.Errorf("config key %q is missing from config-examples/%s", key, exampleFile)
		}
	}
}

func TestSettingsExampleCoversSettingsKeys(t *testing.T) {
	// No exemptions: every key in Settings, including every hook event and every
	// hook transport field, appears in the example.
	assertKeysDocumented(t, "settings.example.yaml", keysOf(config.Settings{}), nil)
}

func TestServerExampleCoversServerKeys(t *testing.T) {
	assertKeysDocumented(t, "server.example.yaml", keysOf(config.ServerConfig{}), nil)
}
