package adapter

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

// The trial home's price list is declared twice: config.ModelPricing is what
// the runtime reads, and Pricing here is what a trial home writes. They cannot
// be one type — config's is tagged for mapstructure and this one for YAML, the
// same reason settingsModel is separate — so this is what keeps them equal.
//
// A rate the runtime gained and the trial home did not is silently dropped: the
// file still parses, the run still prices, and the total is quietly wrong by
// however much that rate was worth. That is worse than failing to price at all,
// because it looks exact.
func TestTheTrialHomeCarriesEveryRateTheRuntimeReads(t *testing.T) {
	want := mapstructureKeys(t, reflect.TypeOf(config.ModelPricing{}))
	got := map[string]bool{}
	for _, key := range yamlKeys(t, reflect.TypeOf(Pricing{})) {
		got[key] = true
	}

	for _, key := range want {
		if !got[key] {
			t.Errorf("adapter.Pricing has no %q; a trial home would drop that rate", key)
		}
	}
	if len(want) != len(got) {
		t.Errorf("adapter.Pricing has %d fields, config.ModelPricing has %d", len(got), len(want))
	}
}

func mapstructureKeys(t *testing.T, typ reflect.Type) []string {
	t.Helper()
	var keys []string
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("mapstructure")
		if tag == "" || tag == "-" {
			t.Fatalf("%s.%s has no mapstructure tag", typ.Name(), typ.Field(i).Name)
		}
		name, _, _ := strings.Cut(tag, ",")
		keys = append(keys, name)
	}
	return keys
}
