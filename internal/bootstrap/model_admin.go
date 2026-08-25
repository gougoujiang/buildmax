package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/gougoujiang/buildmax/internal/config"
	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
	"github.com/gougoujiang/buildmax/internal/infra/db"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/service/llmcatalog"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

// The operator-side half of the managed model catalog. The catalog holds
// provider credentials, so it is edited on the machine that already holds the
// database credentials rather than through a client.
//
// These commands read the same server.yaml the server does, so running them in
// a container or a pod needs no extra configuration.

// ModelCommandUsage is the help text for `buildmax-server model`.
const ModelCommandUsage = `Usage: buildmax-server model <command> [flags]

Commands:
  add       Add a model to the managed catalog
  list      List catalog models
  enable    Re-enable a retired model
  disable   Retire a model without deleting it

Flags for add:
  --name string           Operator-facing name, unique in the deployment (required)
  --api-url string        Upstream base URL (required)
  --api-key string        Upstream credential (required, except for ollama)
  --model string          The provider's own model identifier (required)
  --provider string       Wire protocol: openai_compatible (default), openai,
                          anthropic, ollama (a local daemon, no credential)
  --context-window int    Usable context size; 0 uses the client default
  --call-timeout int      Per-call timeout in seconds; 0 uses the client default
  --max-tokens int        Cap on one response; 0 uses the client default
  --reasoning string      Reasoning effort: off (default), low, medium, high
  --cache-mode string     Prompt cache policy: auto (default), off, force
  --cache-ttl string      Prompt cache retention: provider_default (default),
                          5m, 1h; only where the provider documents it
  --currency string       ISO 4217 code the prices below are quoted in
  --input-price string    Price per million fresh prompt tokens, e.g. 3.00
  --cache-read-price string    Price per million cached prompt tokens read
  --cache-write-price string   Price per million prompt tokens cached
  --output-price string   Price per million generated tokens
  --vision                The upstream accepts image input
  --capabilities string   Comma-separated; defaults to the provider contract

Flags for enable and disable:
  --id string             Model ID (required)

A model is not usable by a team until an alias in server.yaml points at its ID.
An ollama target's --api-url must be reachable from the server, which inside a
container is not the host's localhost. See docs/design/llm-gateway.md.
`

// RunModelCommand executes `buildmax-server model ...`. args excludes the
// "model" word itself.
func RunModelCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, ModelCommandUsage)
		return errors.New("model: a command is required")
	}
	switch args[0] {
	case "add":
		return runModelAdd(ctx, args[1:], out)
	case "list":
		return runModelList(ctx, out)
	case "enable":
		return runModelSetEnabled(ctx, args[1:], out, true)
	case "disable":
		return runModelSetEnabled(ctx, args[1:], out, false)
	case "help", "-h", "--help":
		fmt.Fprint(out, ModelCommandUsage)
		return nil
	default:
		fmt.Fprint(out, ModelCommandUsage)
		return fmt.Errorf("model: unknown command %q", args[0])
	}
}

func runModelAdd(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("model add", flag.ContinueOnError)
	fs.SetOutput(out)
	name := fs.String("name", "", "operator-facing name")
	apiURL := fs.String("api-url", "", "upstream base URL")
	apiKey := fs.String("api-key", "", "upstream credential")
	providerModel := fs.String("model", "", "the provider's own model identifier")
	provider := fs.String("provider", llm.ProviderOpenAICompatible, "wire protocol the upstream speaks")
	contextWindow := fs.Int("context-window", 0, "usable context size")
	callTimeout := fs.Int("call-timeout", 0, "per-call timeout in seconds")
	maxTokens := fs.Int("max-tokens", 0, "cap on one response")
	reasoning := fs.String("reasoning", "", "reasoning effort: off, low, medium, high")
	cacheMode := fs.String("cache-mode", "", "prompt cache policy: auto (default), off, force")
	cacheTTL := fs.String("cache-ttl", "", "prompt cache retention: provider_default (default), 5m, 1h")
	currency := fs.String("currency", "", "ISO 4217 code the prices are quoted in")
	inputPrice := fs.String("input-price", "", "price per million fresh prompt tokens")
	cacheReadPrice := fs.String("cache-read-price", "", "price per million cached prompt tokens read")
	cacheWritePrice := fs.String("cache-write-price", "", "price per million prompt tokens cached")
	outputPrice := fs.String("output-price", "", "price per million generated tokens")
	vision := fs.Bool("vision", false, "the upstream accepts image input")
	capabilities := fs.String("capabilities", "", "comma-separated capability list")
	if err := fs.Parse(args); err != nil {
		return err
	}

	pricing, err := config.ResolvePricing(&config.ModelPricing{
		Currency:          strings.TrimSpace(*currency),
		InputPerMTok:      strings.TrimSpace(*inputPrice),
		CacheReadPerMTok:  strings.TrimSpace(*cacheReadPrice),
		CacheWritePerMTok: strings.TrimSpace(*cacheWritePrice),
		OutputPerMTok:     strings.TrimSpace(*outputPrice),
	})
	if err != nil {
		return fmt.Errorf("model add: %w", err)
	}

	in := coregw.CreateModelInput{
		Name:          strings.TrimSpace(*name),
		ProviderType:  strings.TrimSpace(*provider),
		APIURL:        strings.TrimSpace(*apiURL),
		APIKey:        strings.TrimSpace(*apiKey),
		Model:         strings.TrimSpace(*providerModel),
		ContextWindow: *contextWindow,
		CallTimeout:   *callTimeout,
		MaxTokens:     *maxTokens,
		Reasoning:     strings.TrimSpace(*reasoning),
		CacheMode:     strings.TrimSpace(*cacheMode),
		CacheTTL:      strings.TrimSpace(*cacheTTL),

		Currency:          pricing.Currency,
		InputPerMTok:      pricing.InputPerMTok,
		CacheReadPerMTok:  pricing.CacheReadPerMTok,
		CacheWritePerMTok: pricing.CacheWritePerMTok,
		OutputPerMTok:     pricing.OutputPerMTok,
		Vision:            *vision,
		Capabilities:      parseCapabilityList(*capabilities),
	}

	store, err := openStoreFromConfig(ctx)
	if err != nil {
		return err
	}
	// The catalog holds provider credentials and decides where prompts go, so
	// a change to it is worth a record. The actor is the operator at a shell on
	// the server, which the process cannot name — this command already requires
	// the database credentials, so being on that machine is the authorization.
	svc := &llmcatalog.Service{Models: store, Audit: audit.NewRecorder(store)}
	created, err := svc.Create(ctx, in, coreaudit.OperatorActor())
	if err != nil {
		if errors.Is(err, llmcatalog.ErrNameTaken) {
			return fmt.Errorf("a model named %q already exists", in.Name)
		}
		return commandError("model add", err)
	}

	fmt.Fprintf(out, "Added model %s (%s)\n", created.ID, created.Name)
	fmt.Fprintf(out, "It is available immediately to signed-in users as %q.\n", created.Name)
	fmt.Fprintf(out, "To make it the default when a client names none, set in server.yaml:\n\n"+
		"  llm:\n    default_model: %s\n\nThen restart the server.\n", created.Name)
	return nil
}

func runModelList(ctx context.Context, out io.Writer) error {
	store, err := openStoreFromConfig(ctx)
	if err != nil {
		return err
	}
	models, err := store.ListLLMModels(ctx)
	if err != nil {
		return fmt.Errorf("list models: %w", err)
	}
	if len(models) == 0 {
		fmt.Fprintln(out, "The catalog is empty. Add one with: buildmax-server model add --help")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	// PROVIDER is listed because a catalog can hold more than one wire protocol,
	// and two rows are otherwise indistinguishable from their model identifier.
	fmt.Fprintln(w, "ID\tNAME\tPROVIDER\tMODEL\tAPI URL\tENABLED")
	for _, m := range models {
		enabled := "yes"
		if !m.Enabled {
			enabled = "no"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			m.ID, m.Name, m.ProviderType, m.Model, m.APIURL, enabled)
	}
	return w.Flush()
}

func runModelSetEnabled(ctx context.Context, args []string, out io.Writer, enabled bool) error {
	action := "disable"
	if enabled {
		action = "enable"
	}
	fs := flag.NewFlagSet("model "+action, flag.ContinueOnError)
	fs.SetOutput(out)
	id := fs.String("id", "", "model ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*id) == "" {
		return errors.New("model " + action + ": --id is required")
	}

	store, err := openStoreFromConfig(ctx)
	if err != nil {
		return err
	}
	if err := store.SetLLMModelEnabled(ctx, *id, enabled); err != nil {
		return fmt.Errorf("model %s: %w", action, err)
	}
	auditAction := coreaudit.ModelDisabled
	if enabled {
		auditAction = coreaudit.ModelEnabled
	}
	recordModelAudit(ctx, store, auditAction, *id, "")
	fmt.Fprintf(out, "Model %s is now %sd\n", *id, action)
	return nil
}

// parseCapabilityList defaults to the capability set an OpenAI-compatible
// client already guarantees, matching what the runtime assumes elsewhere.
func parseCapabilityList(s string) []string {
	if strings.TrimSpace(s) == "" {
		out := make([]string, 0, 4)
		for _, c := range llmgateway.BaselineCapabilities() {
			out = append(out, string(c))
		}
		return out
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func openStoreFromConfig(ctx context.Context) (*db.Store, error) {
	sc, err := config.LoadServerConfig()
	if err != nil {
		return nil, fmt.Errorf("server config: %w", err)
	}
	return openStore(ctx, sc.Database)
}

// recordModelAudit writes a catalog change to the audit trail.
//
// The actor is the system rather than a user: this runs from a shell on the
// machine that already holds the database credentials, and inventing a user id
// for it would put a name in the record that nothing verified.
func recordModelAudit(ctx context.Context, store coreaudit.Writer, action, modelID, detail string) {
	audit.NewRecorder(store).Record(ctx, coreaudit.Event{
		ActorType:  coreaudit.ActorSystem,
		ActorID:    "buildmax-server",
		Action:     action,
		TargetType: "llm_model",
		TargetID:   modelID,
		Detail:     detail,
	})
}

// modelAddFlagFor names the flag an input field is set by. The catalog refuses
// input in its own vocabulary, which is right — it has an HTTP caller too — and
// this is where that becomes something an operator can act on without guessing
// which flag it meant.
var modelAddFlagFor = map[string]string{
	"name":           "name",
	"api_url":        "api-url",
	"api_key":        "api-key",
	"model":          "model",
	"context_window": "context-window",
	"call_timeout":   "call-timeout",
	"max_tokens":     "max-tokens",
	"reasoning":      "reasoning",
	"provider_type":  "provider",
	"cache_mode":     "cache-mode",
	"cache_ttl":      "cache-ttl",
}

// commandError renders a catalog refusal the way this command's user reads it.
func commandError(command string, err error) error {
	var invalid *llmcatalog.InvalidField
	if !errors.As(err, &invalid) {
		return fmt.Errorf("%s: %w", command, err)
	}
	flag, ok := modelAddFlagFor[invalid.Field]
	if !ok {
		return fmt.Errorf("%s: %s", command, invalid.Message)
	}
	return fmt.Errorf("%s: --%s %s", command, flag, invalid.Message)
}
