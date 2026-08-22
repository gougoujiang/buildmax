package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/plugin"
	tools "github.com/gougoujiang/buildmax/internal/tool"
)

// invalidPluginExit ends the command non-zero without printing a second time:
// validate has already said what is wrong, next to the thing that is wrong.
func invalidPluginExit() error { return &ExitError{Code: ExitGeneric} }

// writePluginValidatePath validates one directory, which may be anywhere. An
// author testing a checkout before publishing has not installed it yet.
func writePluginValidatePath(w io.Writer, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", path, err)
	}
	data, err := os.ReadFile(filepath.Join(abs, plugin.ManifestFile))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s holds no %s, so it is not a plugin", abs, plugin.ManifestFile)
		}
		return err
	}
	m, findings, err := plugin.Parse(data)
	if err != nil {
		return err
	}
	findings = append(findings, validatePayload(abs, m)...)

	fmt.Fprintf(w, "%s\n", abs)
	if !reportFindings(w, m.Name, findings) {
		return invalidPluginExit()
	}
	return nil
}

// writePluginValidateAll validates every installed plugin, including the ones
// that will not load: those are the ones a user is looking for.
func writePluginValidateAll(w io.Writer, d config.PluginDiscovery) error {
	if len(d.Plugins) == 0 {
		fmt.Fprintf(w, "No plugins installed under %s.\n", d.Dir)
		return writeDiscoveryNotes(w, d)
	}
	failed := false
	for i, p := range d.Plugins {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%s\n", p.Path)
		findings := append([]plugin.Finding(nil), p.Findings...)
		findings = append(findings, validatePayload(p.Path, p.Manifest)...)
		if !reportFindings(w, p.Name(), findings) {
			failed = true
		}
	}
	if err := writeDiscoveryNotes(w, d); err != nil {
		return err
	}
	if failed {
		return invalidPluginExit()
	}
	return nil
}

// validatePayload parses everything the directory contributes, so a manifest
// that is fine but a hooks file that is not still fails validation.
func validatePayload(dir string, m plugin.Manifest) []plugin.Finding {
	p := config.DiscoveredPlugin{Dir: filepath.Base(dir), Path: dir, Manifest: m}
	only := []config.DiscoveredPlugin{p}
	var findings []plugin.Finding

	if _, err := tools.ResolveAgentDefs([]plugin.Source{
		{Dir: filepath.Join(dir, config.PluginAgentsSubdir)},
	}); err != nil {
		findings = append(findings, plugin.Finding{
			Severity: plugin.SeverityError, Field: config.PluginAgentsSubdir,
			Message: err.Error(),
		})
	}
	if _, err := config.ResolveMCPConfig("", only); err != nil {
		findings = append(findings, plugin.Finding{
			Severity: plugin.SeverityError, Field: "mcp.json", Message: err.Error(),
		})
	}
	findings = append(findings, config.ResolvePluginHooks(only).Findings...)
	findings = append(findings, unknownContentFindings(dir)...)
	return findings
}

// unknownContentFindings reports files a plugin ships that this build does not
// read. Silence would let a misplaced directory look like a working feature.
func unknownContentFindings(dir string) []plugin.Finding {
	known := map[string]bool{
		plugin.ManifestFile:       true,
		config.PluginSkillsSubdir: true,
		config.PluginAgentsSubdir: true,
		"mcp.json":                true,
		"hooks.yaml":              true,
		"hooks":                   true,
		"README.md":               true,
		"LICENSE":                 true,
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var findings []plugin.Finding
	for _, e := range entries {
		name := e.Name()
		if known[name] || name == ".git" {
			continue
		}
		findings = append(findings, plugin.Finding{
			Severity: plugin.SeverityWarning, Field: name,
			Message: "not a supported plugin file; this build ignores it",
		})
	}
	return findings
}

// reportFindings prints one plugin's findings and reports whether it would load.
func reportFindings(w io.Writer, name string, findings []plugin.Finding) bool {
	if len(findings) == 0 {
		fmt.Fprintf(w, "  %s is valid.\n", displayName(name))
		return true
	}
	for _, f := range findings {
		fmt.Fprintf(w, "  %s\n", f.String())
	}
	if errs := plugin.Errors(findings); len(errs) > 0 {
		fmt.Fprintf(w, "  %s will not load: %s.\n", displayName(name), countLabel(len(errs), "problem"))
		return false
	}
	fmt.Fprintf(w, "  %s is valid, with warnings.\n", displayName(name))
	return true
}

func countLabel(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func displayName(name string) string {
	if name == "" {
		return "the plugin"
	}
	return name
}
