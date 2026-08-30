package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// .local/ is the one directory a contributor's own configuration lives in.
//
// The same idea used to be spelled three ways in three directories — .env at
// the root, settings.local.yaml beside it, deployment/buildmax-secret.local.yaml
// under deployment/ — each with its own gitignore line and its own paragraph of
// documentation somewhere else. A newcomer had to find all three before a
// checkout would do anything, and nothing named them together.
//
// deployment/compose/.env deliberately stays where it is. Compose reads .env
// from the compose file's own directory, and the quickstart documents raw
// `docker compose` commands that would each need an --env-file to look anywhere
// else. It is also written by generate-env.sh rather than filled in by hand, so
// it is not a file this command could scaffold.
const localDir = ".local"

const (
	localEnvPath      = localDir + "/env"
	localSettingsPath = localDir + "/settings.yaml"
	localSecretPath   = localDir + "/buildmax-secret.yaml"
	localReadmePath   = localDir + "/README.md"

	localSettingsExample = "config-examples/settings.example.yaml"
)

// localFile is one scaffolded file: where it goes, the committed example it is
// copied from, the legacy path it used to have, and what is left to fill in.
//
// The examples stay at their existing homes rather than moving under one
// template directory. Each is already the reviewed, annotated reference for its
// own subsystem, and config-examples/ is packaged into a release archive, which
// a personal-credentials template has no business being in.
type localFile struct {
	path    string
	example string
	legacy  string
	summary string
	fill    string // what a contributor must edit; empty when the copy works as-is
}

func localFiles() []localFile {
	return []localFile{
		{
			path:    localEnvPath,
			example: ".env.example",
			legacy:  ".env",
			summary: "Personal credentials for `" + mk() + "` tasks. Loaded into the environment of every command.",
			fill:    "GITHUB_TOKEN, OPENROUTER_API_KEY, and the DIGITALOCEAN_/SPACES_ entries if you run `" + mk() + " ocean`",
		},
		{
			path:    localSettingsPath,
			example: localSettingsExample,
			legacy:  "settings.local.yaml",
			summary: "Models for `" + mk() + " models` and `" + mk() + " kind seed`. Same shape as BUILDMAX_HOME/settings.yaml, kept separate so no task reads your real runtime configuration.",
			fill:    "a models: entry with your provider api_key — those commands read only that block",
		},
		{
			path:    localSecretPath,
			example: "deployment/buildmax-secret.example.yaml",
			legacy:  "deployment/buildmax-secret.local.yaml",
			summary: "Kubernetes Secret for a deployment you apply by hand: `kubectl apply -f " + localSecretPath + "`. `" + mk() + " kind up` does not read it; that path generates its own throwaway secret.",
			fill:    "BUILDMAX_JWT_SECRET and the provider credentials, before you apply it",
		},
	}
}

// setupLocal creates .local/ and fills it from the committed examples. It is
// idempotent: a file that already exists is left alone, because the thing it
// holds after the first run is credentials.
func setupLocal() error {
	if err := os.MkdirAll(localDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", localDir, err)
	}

	// Only a file this run created from a template is listed as needing work.
	// A moved or already-present file holds whatever its owner put there, and
	// telling them to fill in a key they filled in months ago is noise that
	// makes the same list ignorable on the run where it does matter.
	var fresh []localFile
	moved, kept := 0, 0
	for _, file := range localFiles() {
		wasMoved, err := migrateLocalFile(file)
		if err != nil {
			return err
		}
		if wasMoved {
			moved++
			continue
		}
		if exists(file.path) {
			logf("setup", "%s: already present, left alone", file.path)
			kept++
			continue
		}
		if !exists(file.example) {
			return fmt.Errorf("%s is missing, so %s cannot be created from it", file.example, file.path)
		}
		// 0600 throughout: every one of these holds a credential once it is
		// filled in, and the example it came from does not.
		if err := copyFile(file.example, file.path, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", file.path, err)
		}
		logf("setup", "%s: created from %s", file.path, file.example)
		fresh = append(fresh, file)
	}

	if err := os.WriteFile(localReadmePath, []byte(localReadme()), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", localReadmePath, err)
	}

	fmt.Printf("\n%s\n", localSummary(len(fresh), moved, kept))
	fmt.Printf("It is gitignored in full. Nothing in it is ever committed.\n")
	if len(fresh) > 0 {
		fmt.Println()
		fmt.Println("These are still templates. Fill them in before the commands that read them will work:")
		for _, file := range fresh {
			fmt.Printf("  %-*s  %s\n", localPathWidth(fresh), file.path, file.fill)
		}
	}
	fmt.Printf("\n%s explains what each file is for.\n", localReadmePath)
	return nil
}

func localSummary(fresh, moved, kept int) string {
	var parts []string
	if fresh > 0 {
		parts = append(parts, fmt.Sprintf("%d created from a template", fresh))
	}
	if moved > 0 {
		parts = append(parts, fmt.Sprintf("%d moved here with its contents unchanged", moved))
	}
	if kept > 0 {
		parts = append(parts, fmt.Sprintf("%d already yours and left alone", kept))
	}
	return localDir + " is ready: " + strings.Join(parts, ", ") + "."
}

// localPathWidth keeps the fill-in list aligned without hard-coding a column
// that a longer file name would silently overrun.
func localPathWidth(files []localFile) int {
	width := 0
	for _, file := range files {
		if len(file.path) > width {
			width = len(file.path)
		}
	}
	return width
}

// migrateLocalFile moves a file left at its pre-.local/ path. Without this a
// contributor who already had working credentials would get an empty template
// beside them and no hint that the real file had stopped being read.
func migrateLocalFile(file localFile) (bool, error) {
	if file.legacy == "" || !exists(file.legacy) || exists(file.path) {
		return false, nil
	}
	if err := os.Rename(file.legacy, file.path); err != nil {
		return false, fmt.Errorf("move %s to %s: %w", file.legacy, file.path, err)
	}
	// A rename keeps the old mode, and the paths these came from were not all
	// 0600. Every file in here holds a credential, so the mode is the command's
	// to set whether it created the file or adopted it.
	if err := os.Chmod(file.path, 0o600); err != nil {
		return false, fmt.Errorf("restrict %s: %w", file.path, err)
	}
	logf("setup", "%s: moved from %s, contents unchanged, mode 0600", file.path, file.legacy)
	return true, nil
}

// localStatus is doctor's read-only half of the same question, and is why
// doctor can name `setup local` without repeating the file list.
func localStatus() (missing, legacy []string) {
	for _, file := range localFiles() {
		if !exists(file.path) {
			missing = append(missing, file.path)
		}
		if file.legacy != "" && exists(file.legacy) {
			legacy = append(legacy, file.legacy)
		}
	}
	return missing, legacy
}

func reportLocalConfig() {
	missing, legacy := localStatus()
	switch {
	case len(legacy) > 0:
		fmt.Printf("[WARN] local config: %s now live in %s/ and are no longer read where they are — run `%s setup local` to move them\n",
			strings.Join(legacy, ", "), localDir, mk())
	case len(missing) == 0:
		fmt.Printf("[OK]   local config: %s/ is complete\n", localDir)
	default:
		fmt.Printf("[WARN] local config: %s missing — run `%s setup local` (only needed for the tasks that read them)\n",
			strings.Join(missing, ", "), mk())
	}
}

// localReadme is written into the directory rather than committed to docs/,
// because the reader it is for is standing in .local/ wondering what these
// files are. The task-oriented version lives in docs/reference/configuration.md.
func localReadme() string {
	var b strings.Builder
	b.WriteString("# Local configuration\n\n")
	b.WriteString("Written by `" + mk() + " setup local`. This whole directory is gitignored:\n")
	b.WriteString("it holds real credentials and nothing in it is ever committed.\n\n")
	b.WriteString("Re-running `" + mk() + " setup local` never overwrites a file you have edited.\n")
	b.WriteString("To start one over, delete it and run the command again.\n")
	for _, file := range localFiles() {
		b.WriteString("\n## " + filepath.Base(file.path) + "\n\n")
		b.WriteString(file.summary + "\n\n")
		b.WriteString("Template: `" + file.example + "`\n")
		if file.fill != "" {
			b.WriteString("\nStill to fill in: " + file.fill + "\n")
		}
	}
	b.WriteString("\n## Not here\n\n")
	b.WriteString("`deployment/compose/.env` stays beside its `compose.yaml`, because Compose\n")
	b.WriteString("reads it from that directory and the quickstart runs `docker compose` there\n")
	b.WriteString("directly. `deployment/compose/generate-env.sh` writes it.\n\n")
	b.WriteString("Your real runtime configuration is not here either: the CLI, Desktop, and\n")
	b.WriteString("server read `~/.buildmax/settings.yaml` and `~/.buildmax/server.yaml`.\n")
	b.WriteString("See `docs/reference/configuration.md`.\n")
	return b.String()
}
