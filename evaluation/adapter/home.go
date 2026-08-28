package adapter

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

// ModelAccess is how a trial reaches its model and what that costs. It travels
// beside the subject rather than inside it, for two different reasons that both
// end in the same place.
//
// The credential is section 8.2: a stored trial result must never become a
// credential store. The prices are section 12: they change what a run reports
// spending without changing anything it did, so two subjects at different
// prices are the same subject and must compare as one.
type ModelAccess struct {
	APIURL string
	APIKey string
	// Pricing lets a run report what it spent. Absent, the trial reports its
	// cost as unavailable rather than as zero — BuildMax does not know what any
	// provider charges, and guessing would be worse than silence.
	Pricing *Pricing
}

// Pricing is what one model charges, in the shape settings.yaml takes. Rates
// are strings for the reason internal/config keeps them as strings: a price is
// a decimal, and parsing one through a float loses the last digits of a rate
// quoted in millionths.
type Pricing struct {
	Currency          string `yaml:"currency"`
	InputPerMTok      string `yaml:"input_per_mtok,omitempty"`
	CacheReadPerMTok  string `yaml:"cache_read_per_mtok,omitempty"`
	CacheWritePerMTok string `yaml:"cache_write_per_mtok,omitempty"`
	OutputPerMTok     string `yaml:"output_per_mtok,omitempty"`
}

// settingsFile is the subset of settings.yaml a trial home needs.
//
// It is declared here rather than reused from internal/config because that
// struct is tagged for mapstructure, not YAML, so marshalling it writes keys
// the loader does not read. Declaring the subset also states what a subject
// controls: a trial home carries a model and nothing else, which is what stops
// a contributor's own hooks, plugins, or permissions from changing the thing
// being measured.
type settingsFile struct {
	LogLevel string          `yaml:"log_level"`
	Models   []settingsModel `yaml:"models"`
}

type settingsModel struct {
	Model         string   `yaml:"model"`
	Name          string   `yaml:"name"`
	Provider      string   `yaml:"provider,omitempty"`
	APIURL        string   `yaml:"api_url,omitempty"`
	APIKey        string   `yaml:"api_key,omitempty"`
	ContextWindow int      `yaml:"context_window,omitempty"`
	MaxTokens     int      `yaml:"max_tokens,omitempty"`
	Reasoning     string   `yaml:"reasoning,omitempty"`
	Pricing       *Pricing `yaml:"pricing,omitempty"`
}

// WriteHome builds the BUILDMAX_HOME one trial runs under, from the subject
// alone.
//
// Building it rather than reusing the contributor's home is what makes the
// result attributable. Section 2.1 lists local settings, hooks, plugins, and
// permissions among the things that silently change what a benchmark measures;
// a home containing only the subject's model has none of them to inherit, and
// a plugin installed on the machine cannot reach a run that never looks there.
func WriteHome(dir string, subject contract.SubjectManifest, cred ModelAccess) error {
	if subject.Model.Target == "" {
		return fmt.Errorf("subject %q names no model target", subject.Name)
	}
	if subject.Model.Transport == "buildmax" {
		// A managed-gateway subject needs a server URL, a team, and a login the
		// CLI adapter has no way to establish. Refusing is better than writing
		// a direct entry that quietly measures a different transport than the
		// manifest claims.
		return fmt.Errorf("subject %q uses the managed gateway, which the CLI adapter does not reach yet", subject.Name)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create trial home: %w", err)
	}

	settings := settingsFile{
		// Trial stderr is diagnostic output the bundle keeps; info-level logs
		// would bury a real failure in ordinary progress.
		LogLevel: "error",
		Models: []settingsModel{{
			Model:         subject.Model.Target,
			Name:          subject.Model.Target,
			Provider:      subject.Model.Transport,
			APIURL:        cred.APIURL,
			APIKey:        cred.APIKey,
			ContextWindow: subject.Model.ContextWindow,
			MaxTokens:     subject.Model.MaxOutput,
			Reasoning:     subject.Model.Reasoning,
			Pricing:       cred.Pricing,
		}},
	}

	data, err := yaml.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	path := filepath.Join(dir, "settings.yaml")
	// 0o600: the file holds a provider credential, and a trial home lives in a
	// shared temporary directory.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
}

// TracesDir is where the runtime writes durable traces under a home. The
// adapter locates a trial's trace from the run's reported path rather than by
// rebuilding this layout; the constant exists for the fallback in Run, which
// says so where it uses it.
const TracesDir = "traces"
