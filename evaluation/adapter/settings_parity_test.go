package adapter

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// harborSettingsModule is the Python that writes the same file this package's
// settingsModel does, for a trial that runs inside a Harbor container.
const harborSettingsModule = "../harbor/src/buildmax_harbor/settings.py"

// The trial home's shape has two writers in two languages: this package builds
// it for a local CLI trial, and the Harbor adapter renders it inside a task
// container. They cannot share code across that boundary, so this is what keeps
// them from drifting.
//
// Drift here is quiet and expensive. A key the Python side spells wrongly is
// not an error — the loader ignores it — so the trial runs with the field
// unset, and the result is attributed to a subject that was never configured
// the way the manifest claims.
func TestHarborSettingsRenderTheSameKeys(t *testing.T) {
	body, err := os.ReadFile(filepath.FromSlash(harborSettingsModule))
	if err != nil {
		t.Fatalf("read the Harbor settings renderer: %v", err)
	}
	rendered := string(body)

	keys := yamlKeys(t, reflect.TypeOf(settingsModel{}))
	keys = append(keys, yamlKeys(t, reflect.TypeOf(Pricing{}))...)
	for _, key := range keys {
		// The Python writes each key as a literal `key: ` prefix, so this is
		// the string it would have to change to break the pair. The rates sit
		// inside a nested block and are written from a tuple rather than one
		// line each, so a bare name counts for them.
		if strings.Contains(rendered, key+": ") || strings.Contains(rendered, `"`+key+`"`) {
			continue
		}
		t.Errorf("%s writes no %q key, which this package's trial home carries",
			harborSettingsModule, key)
	}

	// The other direction: a key the Python writes that Go does not know is a
	// setting the local adapter would never apply, so the two surfaces would be
	// measuring differently configured subjects.
	known := map[string]bool{}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(settingsModel{}),
		reflect.TypeOf(settingsFile{}),
		reflect.TypeOf(Pricing{}),
	} {
		for _, key := range yamlKeys(t, typ) {
			known[key] = true
		}
	}
	for _, line := range strings.Split(rendered, "\n") {
		key, ok := renderedKey(line)
		if !ok || known[key] {
			continue
		}
		t.Errorf("%s writes %q, which is not a field of the trial home this package builds",
			harborSettingsModule, key)
	}
}

// yamlKeys returns a struct's yaml tag names, without their options.
func yamlKeys(t *testing.T, typ reflect.Type) []string {
	t.Helper()
	var keys []string
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("yaml")
		if tag == "" || tag == "-" {
			t.Fatalf("%s.%s has no yaml tag, so nothing can be held to it",
				typ.Name(), typ.Field(i).Name)
		}
		name, _, _ := strings.Cut(tag, ",")
		keys = append(keys, name)
	}
	return keys
}

// renderedKey pulls the settings key out of one line of the Python renderer's
// output template, which builds lines shaped like `    api_url: {...}`.
func renderedKey(line string) (string, bool) {
	const marker = `: {`
	at := strings.Index(line, marker)
	if at < 0 {
		return "", false
	}
	// The key is the last bare word before the colon, after the indentation,
	// the quote, and any list dash the template carries.
	head := line[:at]
	head = strings.Trim(head, `"' `)
	head = strings.TrimPrefix(head, "- ")
	if head == "" || strings.ContainsAny(head, " \t(),") {
		return "", false
	}
	return head, true
}
