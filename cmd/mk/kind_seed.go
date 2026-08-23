package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// `kind seed` puts a contributor's own models into the local cluster's catalog
// so the CLI and Desktop can exercise the managed transport against real
// inference, instead of needing a hosted deployment to point at.
//
// The cluster's own inference is left alone: conversation.model and the worker
// keep answering from the in-cluster mock, so `kind smoke` stays deterministic
// and costs nothing after a seed. What this adds is `llm.aliases`, the one
// binding that has to name a catalog ID — and an ID exists only once the row
// does, which is why this runs after the deployment rather than inside it.
//
// Rows are added through `buildmax-server model add` rather than by writing the
// table. The command already owns public ID generation, the default capability
// set, and field validation; a second copy of those in mk would be a copy that
// nothing keeps in step with the schema.

const (
	// kindSeedBaseConfig is the ConfigMap body the aliases are appended to. It
	// is the direct variant `kind up` applies: seeding grants the deployment's
	// teams a model, it does not switch how the deployment itself infers.
	kindSeedBaseConfig = "deployment/smoke/server.kind.yaml"

	// kindHostAddress is how a pod reaches a daemon on this machine. Docker
	// Desktop resolves it inside the cluster, which is what makes a local Ollama
	// usable from the deployment; on Linux the equivalent is the bridge gateway,
	// so a rewrite says what it did rather than doing it silently.
	kindHostAddress = "host.docker.internal"
)

// addedModelPattern reads the ID back out of `model add`. The alias has to name
// it and nothing else prints it, so a change to that line is a change to this.
var addedModelPattern = regexp.MustCompile(`(?m)^Added model (\S+) \((.*)\)$`)

// publicIDPattern is the text form of a public ID: 20 lowercase base32
// characters. Catalog output is parsed by column, and this is what tells a data
// row from a log line that landed in the same stream.
var publicIDPattern = regexp.MustCompile(`^[a-z2-7]{20}$`)

// kindSeedEntry is one settings.local.yaml model as the cluster will hold it.
type kindSeedEntry struct {
	// alias is the name a team calls it by, and what a managed model entry in a
	// contributor's settings.yaml puts in its `model` field. It is derived from
	// the provider's own model id rather than the display name, so it is unique
	// in the file and recognisable on both sides.
	alias string
	// source is the settings.local.yaml model id the alias was derived from, so
	// the printed entries say which model each one is.
	source string
	// name is the catalog row's operator-facing name, unique in the deployment.
	name string
	// id is the catalog ID the alias binds to.
	id string
}

func kindSeed() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	cluster := kindClusterName()
	exists, err := kindClusterExists(cluster)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("kind cluster %q does not exist; run %s kind up", cluster, mk())
	}

	configured, err := readLocalModels()
	if err != nil {
		return err
	}
	direct := directSettingsModels(configured)
	if len(direct) == 0 {
		return fmt.Errorf("%s configures no provider model to seed; a managed entry names a team alias, which is what this command creates", localSettingsPath)
	}

	target := kindSmokeTarget()
	existing, err := kindCatalogIDs(target)
	if err != nil {
		return err
	}

	entries, err := seedKindCatalog(target, direct, existing)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return errors.New("no model could be seeded")
	}

	fmt.Println("\nGranting the deployment's teams these models...")
	if err := applyKindSeedConfig(entries); err != nil {
		return err
	}
	if err := kindKubectl("rollout", "restart", "deployment/buildmax-server", "-n", "buildmax"); err != nil {
		return err
	}
	if err := kindKubectl("rollout", "status", "deployment/buildmax-server", "-n", "buildmax", "--timeout=180s"); err != nil {
		return err
	}

	return printKindSeedUsage(entries)
}

// directSettingsModels drops the entries that already call a gateway. Such an
// entry names a team alias rather than an upstream, so there is nothing in it
// for a catalog to hold.
func directSettingsModels(models []settingsModel) []settingsModel {
	out := make([]settingsModel, 0, len(models))
	for _, m := range models {
		if m.isManaged() {
			fmt.Printf("Skipping %s: it is already a managed entry.\n", m.id)
			continue
		}
		out = append(out, m)
	}
	return out
}

// seedKindCatalog adds every model that is not already there, and reuses the ID
// of every one that is.
//
// A name already in the catalog is left untouched rather than updated: the add
// command does not update, and silently replacing a row an operator created by
// hand would be worse than saying so. Changing a seeded model means renaming it
// here, or rebuilding the cluster.
func seedKindCatalog(target smokeTarget, models []settingsModel, existing map[string]string) ([]kindSeedEntry, error) {
	entries := make([]kindSeedEntry, 0, len(models))
	claimed := make(map[string]string, len(models))
	for _, m := range models {
		name := m.name
		if name == "" {
			name = m.id
		}
		alias := aliasFromModelID(m.id)
		if alias == "" {
			fmt.Printf("Skipping %q: %q yields no usable alias.\n", name, m.id)
			continue
		}
		if other, taken := claimed[alias]; taken {
			fmt.Printf("Skipping %q: %q already claims the alias %s.\n", name, other, alias)
			continue
		}

		id, known := existing[name]
		if known {
			fmt.Printf("  %s is already in the catalog as %s\n", name, id)
		} else {
			added, err := addKindCatalogModel(target, m, name)
			if err != nil {
				return nil, err
			}
			id = added
			fmt.Printf("  %s added as %s\n", name, id)
		}
		claimed[alias] = name
		entries = append(entries, kindSeedEntry{alias: alias, source: m.id, name: name, id: id})
	}
	return entries, nil
}

// aliasFromModelID turns a provider model id into a name the server can
// actually be configured with.
//
// server.yaml is read through viper, which treats a dot as a path separator: an
// alias of "openai/gpt-5.6-luna" arrives as a nested map rather than a name and
// the server refuses to start. Quoting the key does not help — the split
// happens after YAML parsing — so every character outside [a-z0-9] becomes a
// dash. The vendor prefix is kept, because two vendors serving the same model
// name are what would otherwise collide.
func aliasFromModelID(id string) string {
	var b strings.Builder
	dashed := false
	for _, r := range strings.ToLower(id) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dashed = false
			continue
		}
		if !dashed && b.Len() > 0 {
			b.WriteByte('-')
			dashed = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func addKindCatalogModel(target smokeTarget, m settingsModel, name string) (string, error) {
	args := []string{"model", "add", "--name", name, "--model", m.id, "--api-url", kindReachableURL(m.apiURL, name)}
	// An empty flag is not the same as an absent one here: the add command
	// defaults --provider to openai_compatible and rejects an empty value, so an
	// entry that names no protocol must not pass the flag at all.
	if m.provider != "" {
		args = append(args, "--provider", m.provider)
	}
	if m.apiKey != "" {
		args = append(args, "--api-key", m.apiKey)
	}
	if m.contextWindow > 0 {
		args = append(args, "--context-window", strconv.Itoa(m.contextWindow))
	}
	if m.callTimeout > 0 {
		args = append(args, "--call-timeout", strconv.Itoa(m.callTimeout))
	}
	if m.maxTokens > 0 {
		args = append(args, "--max-tokens", strconv.Itoa(m.maxTokens))
	}
	if m.reasoning != "" {
		args = append(args, "--reasoning", m.reasoning)
	}
	if m.promptCache {
		args = append(args, "--prompt-cache")
	}
	if m.vision {
		args = append(args, "--vision")
	}

	output, err := target.admin(args...)
	if err != nil {
		return "", fmt.Errorf("add %q to the catalog: %w", name, err)
	}
	match := addedModelPattern.FindStringSubmatch(output)
	if match == nil {
		return "", fmt.Errorf("add %q to the catalog: the command printed no model ID: %s", name, output)
	}
	return match[1], nil
}

// kindReachableURL rewrites an address that means "this machine" into one a pod
// can reach. A local runtime is the whole point of seeding for some
// contributors, and its settings.local.yaml entry necessarily points at
// loopback, which inside a pod is the pod itself.
func kindReachableURL(rawURL, name string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return rawURL
	}
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0":
	default:
		return rawURL
	}
	host := kindHostAddress
	if port := parsed.Port(); port != "" {
		host = host + ":" + port
	}
	parsed.Host = host
	rewritten := parsed.String()
	fmt.Printf("  %s points at this machine; the cluster will use %s\n", name, rewritten)
	fmt.Printf("    On Linux that name does not resolve — use the bridge gateway from `docker network inspect kind`\n" +
		"    and start the daemon with OLLAMA_HOST=0.0.0.0.\n")
	return rewritten
}

// kindCatalogIDs reads the catalog as a name-to-ID map.
//
// `model list` writes an aligned table, so the columns are found from the
// header rather than by splitting on whitespace: a model name may contain
// spaces. Anything that is not a data row — a log line sharing the stream, the
// empty-catalog notice — is skipped rather than guessed at.
func kindCatalogIDs(target smokeTarget) (map[string]string, error) {
	output, err := target.admin("model", "list")
	if err != nil {
		return nil, fmt.Errorf("read the catalog: %w", err)
	}
	return parseCatalogIDs(output), nil
}

func parseCatalogIDs(output string) map[string]string {
	ids := make(map[string]string)
	nameStart, nameEnd := -1, -1
	for _, raw := range strings.Split(output, "\n") {
		line := []rune(strings.TrimRight(raw, " \r"))
		if nameStart < 0 {
			nameStart, nameEnd = catalogNameColumn(string(line))
			continue
		}
		if len(line) <= nameStart {
			continue
		}
		id := strings.TrimSpace(string(line[:min(nameStart, len(line))]))
		if !publicIDPattern.MatchString(id) {
			continue
		}
		name := strings.TrimSpace(string(line[nameStart:min(nameEnd, len(line))]))
		if name == "" {
			continue
		}
		ids[name] = id
	}
	return ids
}

// catalogNameColumn locates the NAME column in the table header, returning
// (-1, -1) for any other line.
func catalogNameColumn(line string) (int, int) {
	if !strings.HasPrefix(line, "ID") || !strings.Contains(line, "ENABLED") {
		return -1, -1
	}
	start := strings.Index(line, "NAME")
	provider := strings.Index(line, "PROVIDER")
	if start < 0 || provider < 0 || provider <= start {
		return -1, -1
	}
	// Index counts bytes and the data rows are sliced as runes, so both ends are
	// measured the same way. The header is ASCII, but a name above it need not be.
	return len([]rune(line[:start])), len([]rune(line[:provider]))
}

// applyKindSeedConfig rewrites the server ConfigMap with an alias per seeded
// model. Aliases are startup configuration, unlike the catalog itself, which is
// why this is the step that costs a restart.
func applyKindSeedConfig(entries []kindSeedEntry) error {
	base, err := os.ReadFile(kindSeedBaseConfig)
	if err != nil {
		return fmt.Errorf("read %s: %w", kindSeedBaseConfig, err)
	}
	rendered := string(base) + renderKindSeedAliases(entries)

	// The ConfigMap is created from a file, and the body only exists in memory,
	// so it is staged next to the source it was rendered from.
	staged, err := os.CreateTemp("", "buildmax-kind-seed-*.yaml")
	if err != nil {
		return fmt.Errorf("stage the seeded server config: %w", err)
	}
	path := staged.Name()
	defer func() { _ = os.Remove(path) }()
	if _, err := staged.WriteString(rendered); err != nil {
		_ = staged.Close()
		return fmt.Errorf("stage the seeded server config: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("stage the seeded server config: %w", err)
	}
	return applyKindSmokeConfigFrom(path)
}

func renderKindSeedAliases(entries []kindSeedEntry) string {
	var b strings.Builder
	b.WriteString("\n# Aliases for the models in " + localSettingsPath + ", written by `" + mk() + " kind seed`.\n")
	b.WriteString("# `" + mk() + " kind up` renders this file without them again.\n")
	b.WriteString("llm:\n")
	b.WriteString("  default_alias: " + entries[0].alias + "\n")
	b.WriteString("  aliases:\n")
	for _, entry := range entries {
		b.WriteString("    " + entry.alias + ": " + entry.id + "  # " + entry.source + "\n")
	}
	return b.String()
}

// printKindSeedUsage prints the managed model entries a contributor pastes into
// their own settings.yaml. A managed entry needs a team, and the team is the
// personal one of the account `kind up` and `kind info` already sign in as.
func printKindSeedUsage(entries []kindSeedEntry) error {
	target := kindSmokeTarget()
	client := &http.Client{Timeout: 10 * time.Second}
	ctx := context.Background()
	if err := waitForHTTP(ctx, client, target.apiBase+"/healthz", 90*time.Second); err != nil {
		return err
	}
	_, teamID, err := smokeSignIn(ctx, client, target, smokeEmail)
	if err != nil {
		return fmt.Errorf("find the team to use these models: %w", err)
	}

	fmt.Printf("\nSeeded %d model(s) into cluster %s.\n", len(entries), kindClusterName())
	fmt.Printf("The cluster's own inference is untouched: Portal conversations and task runs\n"+
		"still answer from the mock, so `%s kind smoke` stays free and deterministic.\n", mk())
	fmt.Printf("\nAdd these to your BUILDMAX_HOME/settings.yaml to drive them from the CLI or Desktop:\n\n")
	fmt.Println("models:")
	for _, entry := range entries {
		fmt.Printf("  - model: %s  # %s\n", entry.alias, entry.source)
		fmt.Printf("    name: %s (kind)\n", entry.name)
		fmt.Printf("    transport: buildmax\n")
		fmt.Printf("    server_url: %s\n", target.apiBase)
		fmt.Printf("    team_id: %s\n", teamID)
	}
	fmt.Printf("\nThe credential comes from the login, not the file: sign in first with\n")
	fmt.Printf("  buildmax login --server %s   (as %s)\n", target.apiBase, smokeEmail)
	fmt.Printf("Run `%s kind info` for a single-use code, then `buildmax models --team %s` to check.\n", mk(), teamID)
	return nil
}
