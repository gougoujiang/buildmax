package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/plugin"
	inspect "github.com/gougoujiang/buildmax/internal/service/plugininspect"
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
	fmt.Fprintf(w, "%s\n", abs)
	ok, err := reportPackage(w, abs)
	if err != nil {
		return err
	}
	if !ok {
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
		ok, err := reportPackage(w, p.Path)
		if err != nil {
			fmt.Fprintf(w, "  error: %v\n", err)
			failed = true
			continue
		}
		if !ok {
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

// reportPackage inspects one directory and prints what it found.
//
// The inspection is the same one publication will run, so a plugin that passes
// here cannot fail there for a reason this command could have named.
func reportPackage(w io.Writer, dir string) (bool, error) {
	pkg, err := inspect.Dir(os.DirFS(dir))
	if err != nil {
		return false, err
	}
	for _, f := range pkg.Findings {
		fmt.Fprintf(w, "  %s\n", f.String())
	}
	name := displayName(pkg.Manifest.Name)
	if errs := plugin.Errors(pkg.Findings); len(errs) > 0 {
		fmt.Fprintf(w, "  %s will not load: %s.\n", name, countLabel(len(errs), "problem"))
		return false, nil
	}
	if len(pkg.Findings) > 0 {
		fmt.Fprintf(w, "  %s is valid, with warnings.\n", name)
		return true, nil
	}
	fmt.Fprintf(w, "  %s is valid.\n", name)
	return true, nil
}

func countLabel(n int, noun string) string {
	return countLabelPlural(n, noun, noun+"s")
}

// countLabelPlural is countLabel for a noun whose plural is not the singular
// plus an s. "2 memorys" is the shape this exists to prevent.
func countLabelPlural(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
}

func displayName(name string) string {
	if name == "" {
		return "the plugin"
	}
	return name
}
