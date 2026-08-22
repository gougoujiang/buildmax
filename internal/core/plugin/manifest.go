// Package plugin owns the plugin manifest: its format, its rules, and the
// version arithmetic that decides whether a build may run a release.
//
// It is pure domain code with no I/O because both sides of distribution need
// it and they cannot share anything above core: internal/server may not import
// internal/config (see internal/architecture), so discovery and publication
// would otherwise each grow their own parser and drift apart.
package plugin

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestFile is the file whose presence makes a directory a plugin.
const ManifestFile = "plugin.yaml"

const maxNameLen = 64

// Manifest is a parsed plugin.yaml. Only Name is required to load a plugin;
// Version is additionally required to publish one.
type Manifest struct {
	Name        string
	Version     string
	Description string

	DisplayName string
	Homepage    string
	Maintainer  string
	License     string

	MinBuildmaxVersion string

	// Env is the declared environment contract, in file order.
	Env []EnvVar

	// Unknown lists top-level keys this build did not recognise, in file
	// order. They are kept rather than dropped so `plugin validate` can show a
	// misspelling that would otherwise be invisible.
	Unknown []string
}

// EnvVar is one declared environment variable. It carries a name and prose and
// never a value.
type EnvVar struct {
	Name        string
	Description string
	// Required defaults to true: declaring a variable at all normally means the
	// plugin wants it.
	Required bool
}

// EnvVarByName returns the declared entry for a variable name.
func (m Manifest) EnvVarByName(name string) (EnvVar, bool) {
	for _, e := range m.Env {
		if e.Name == name {
			return e, true
		}
	}
	return EnvVar{}, false
}

// DisplayTitle is what a catalog or a Desktop list shows.
func (m Manifest) DisplayTitle() string {
	if m.DisplayName != "" {
		return m.DisplayName
	}
	return m.Name
}

var (
	namePattern    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ValidateName reports why a string cannot be a plugin name. The name is also a
// directory name under <BUILDMAX_HOME>/plugins, so anything path-shaped is out.
func ValidateName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("plugin name is required")
	case len(name) > maxNameLen:
		return fmt.Errorf("plugin name %q is longer than %d characters", name, maxNameLen)
	case !namePattern.MatchString(name):
		return fmt.Errorf("plugin name %q must be lowercase alphanumeric words joined by single hyphens", name)
	default:
		return nil
	}
}

// Parse reads a plugin.yaml.
//
// The returned error means the bytes are not a manifest document at all, which
// is the one case where no partial result exists. Every rule violation is a
// findings entry instead, so `plugin validate` can report a whole file rather
// than the first thing wrong with it. Callers that must decide whether to load
// the plugin check HasErrors.
func Parse(data []byte) (Manifest, []Finding, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Manifest{}, nil, fmt.Errorf("parse %s: %w", ManifestFile, err)
	}

	var m Manifest
	var findings []Finding

	root := documentRoot(&doc)
	if root == nil {
		// An empty file is not a syntax error, but it has no name either, so
		// it falls through to the required-field check below.
		findings = append(findings, requiredNameFinding(0))
		return m, findings, nil
	}
	if root.Kind != yaml.MappingNode {
		return Manifest{}, nil, fmt.Errorf("parse %s: top level must be a mapping", ManifestFile)
	}

	seen := map[string]int{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, val := root.Content[i], root.Content[i+1]
		name := key.Value
		if prev, dup := seen[name]; dup {
			findings = append(findings, Finding{
				Severity: SeverityError, Field: name, Line: key.Line,
				Message: fmt.Sprintf("duplicate key, first defined on line %d", prev),
			})
			continue
		}
		seen[name] = key.Line

		switch name {
		case "name":
			m.Name, findings = decodeString(val, name, findings)
		case "version":
			m.Version, findings = decodeString(val, name, findings)
		case "description":
			m.Description, findings = decodeString(val, name, findings)
		case "display_name":
			m.DisplayName, findings = decodeString(val, name, findings)
		case "homepage":
			m.Homepage, findings = decodeString(val, name, findings)
		case "maintainer":
			m.Maintainer, findings = decodeString(val, name, findings)
		case "license":
			m.License, findings = decodeString(val, name, findings)
		case "min_buildmax_version":
			m.MinBuildmaxVersion, findings = decodeString(val, name, findings)
		case "env":
			var envFindings []Finding
			m.Env, envFindings = parseEnv(val)
			findings = append(findings, envFindings...)
		default:
			m.Unknown = append(m.Unknown, name)
			findings = append(findings, Finding{
				Severity: SeverityWarning, Field: name, Line: key.Line,
				Message: "unknown field, ignored by this build",
			})
		}
	}

	findings = append(findings, validateValues(m, seen)...)
	return m, findings, nil
}

// validateValues checks the fields that have rules beyond being a string.
func validateValues(m Manifest, lines map[string]int) []Finding {
	var findings []Finding
	if err := ValidateName(m.Name); err != nil {
		if m.Name == "" {
			findings = append(findings, requiredNameFinding(lines["name"]))
		} else {
			findings = append(findings, Finding{
				Severity: SeverityError, Field: "name", Line: lines["name"], Message: err.Error(),
			})
		}
	}
	if m.Version != "" {
		if _, err := ParseVersion(m.Version); err != nil {
			findings = append(findings, Finding{
				Severity: SeverityError, Field: "version", Line: lines["version"], Message: err.Error(),
			})
		}
	}
	if m.MinBuildmaxVersion != "" {
		if _, err := ParseVersion(m.MinBuildmaxVersion); err != nil {
			findings = append(findings, Finding{
				Severity: SeverityError, Field: "min_buildmax_version", Line: lines["min_buildmax_version"],
				Message: err.Error() + "; a single lower bound, not a range",
			})
		}
	}
	return findings
}

func requiredNameFinding(line int) Finding {
	return Finding{
		Severity: SeverityError, Field: "name", Line: line,
		Message: "plugin name is required",
	}
}

// parseEnv reads the env block. Unlike the top level, an unrecognised key here
// is an error: this is the one place where accepting a field nobody reads could
// let a checked-in secret pass validation.
func parseEnv(node *yaml.Node) ([]EnvVar, []Finding) {
	if node.Kind != yaml.MappingNode {
		return nil, []Finding{{
			Severity: SeverityError, Field: "env", Line: node.Line,
			Message: "must be a mapping of variable name to declaration",
		}}
	}

	var vars []EnvVar
	var findings []Finding
	seen := map[string]int{}

	for i := 0; i+1 < len(node.Content); i += 2 {
		key, val := node.Content[i], node.Content[i+1]
		field := "env." + key.Value
		if prev, dup := seen[key.Value]; dup {
			findings = append(findings, Finding{
				Severity: SeverityError, Field: field, Line: key.Line,
				Message: fmt.Sprintf("duplicate variable, first declared on line %d", prev),
			})
			continue
		}
		seen[key.Value] = key.Line

		if !envNamePattern.MatchString(key.Value) {
			findings = append(findings, Finding{
				Severity: SeverityError, Field: field, Line: key.Line,
				Message: "not a valid environment variable name",
			})
			continue
		}
		if val.Kind != yaml.MappingNode {
			findings = append(findings, Finding{
				Severity: SeverityError, Field: field, Line: val.Line,
				Message: "must be a mapping with description and optional required",
			})
			continue
		}

		e := EnvVar{Name: key.Value, Required: true}
		entrySeen := map[string]bool{}
		for j := 0; j+1 < len(val.Content); j += 2 {
			ek, ev := val.Content[j], val.Content[j+1]
			if entrySeen[ek.Value] {
				findings = append(findings, Finding{
					Severity: SeverityError, Field: field + "." + ek.Value, Line: ek.Line,
					Message: "duplicate key",
				})
				continue
			}
			entrySeen[ek.Value] = true

			switch ek.Value {
			case "description":
				e.Description, findings = decodeString(ev, field+".description", findings)
			case "required":
				if err := ev.Decode(&e.Required); err != nil {
					findings = append(findings, Finding{
						Severity: SeverityError, Field: field + ".required", Line: ev.Line,
						Message: "must be true or false",
					})
					e.Required = true
				}
			default:
				findings = append(findings, Finding{
					Severity: SeverityError, Field: field, Line: ek.Line,
					Message: fmt.Sprintf("unknown key %q; an entry declares only description and required, "+
						"and a manifest must never carry a value", ek.Value),
				})
			}
		}
		vars = append(vars, e)
	}
	return vars, findings
}

// decodeString takes any scalar's text rather than insisting on a YAML string,
// so `version: 1.2` is reported as a malformed version instead of the wrong
// type — which is what the author actually got wrong.
func decodeString(node *yaml.Node, field string, findings []Finding) (string, []Finding) {
	if node.Kind != yaml.ScalarNode {
		return "", append(findings, Finding{
			Severity: SeverityError, Field: field, Line: node.Line,
			Message: "must be a single value, not a list or mapping",
		})
	}
	if node.Tag == "!!null" {
		return "", findings
	}
	return strings.TrimSpace(node.Value), findings
}

func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			return nil
		}
		return doc.Content[0]
	}
	if doc.Kind == 0 {
		return nil
	}
	return doc
}
