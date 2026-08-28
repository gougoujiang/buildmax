package harbor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const adapterModule = "src/buildmax_harbor/agent.py"

// The subject tuple crosses a language boundary: the Python adapter resolves it
// inside a container and records it on the trial, and this package reads it
// back out. Nothing in either language checks that they agree on the names.
//
// Drift here is silent and expensive in exactly the way the trial-home key set
// is. A key the adapter spells differently is not an error — the map lookup
// simply misses — so the importer either refuses every trial with "produced by
// an adapter older than this importer" or, worse for the fields that are not
// required, files a subject with an unrecorded reasoning effort or an
// unresolved sandbox boundary.
func TestTheAdapterRecordsEverySubjectFactTheImporterReads(t *testing.T) {
	body, err := os.ReadFile(filepath.FromSlash(adapterModule))
	if err != nil {
		t.Fatalf("read the adapter: %v", err)
	}
	source := string(body)
	for _, key := range subjectMetadataKeys {
		// The adapter writes each one as a dict key, so this is the literal it
		// would have to change to break the pair.
		if !strings.Contains(source, `"`+key+`":`) {
			t.Errorf("%s records no %q; the importer reads it from every trial", adapterModule, key)
		}
	}
}

// The other direction, for the fixtures. A job fixture is written by hand, so
// it can keep passing after the adapter stops producing a field — which would
// leave the conversion tests green against evidence no real run produces.
func TestTheJobFixtureCarriesTheSameSubjectFacts(t *testing.T) {
	trials, err := LoadJob(jobFixture)
	if err != nil {
		t.Fatalf("LoadJob: %v", err)
	}
	meta := metadataOf(trials[0])
	if meta == nil {
		t.Fatal("the first fixture trial records no agent metadata")
	}
	for _, key := range subjectMetadataKeys {
		if _, ok := meta[key]; !ok {
			t.Errorf("the job fixture records no %q, so the importer is tested against evidence no run produces", key)
		}
	}
}
