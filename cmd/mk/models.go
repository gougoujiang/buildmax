package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// localSettingsPath is a repo-root, gitignored file in the same shape as
// BUILDMAX_HOME/settings.yaml (internal/config.Settings) — config-examples/
// localSettingsExample already documents that shape — kept separate from the
// real one so `./make models` never reads or depends on a contributor's
// actual runtime configuration.
const (
	localSettingsPath    = "settings.local.yaml"
	localSettingsExample = "config-examples/settings.example.yaml"

	openRouterModelsURL = "https://openrouter.ai/api/v1/models"
)

func cmdModels(args []string) error {
	if len(args) == 0 {
		return usageErrorf("models", "models needs an action")
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return usageErrorf("models", "models list takes no arguments")
		}
		return modelsList()
	case "info":
		switch len(args) {
		case 1:
			return modelsInfoLocal()
		case 2:
			if args[1] == "" {
				return usageErrorf("models", "models info needs a model or search term")
			}
			return modelsInfo(args[1])
		default:
			return usageErrorf("models", "models info takes at most one search term")
		}
	case "check":
		if len(args) != 1 {
			return usageErrorf("models", "models check takes no arguments")
		}
		return modelsCheck()
	default:
		return usageErrorf("models", "unknown models action: %s", args[0])
	}
}

// modelsList prints what is configured locally. It never reaches the
// network: "what did I set up" and "what does OpenRouter say" are different
// questions, and the first should never wait on the second.
func modelsList() error {
	configured, err := readLocalModels()
	if err != nil {
		return err
	}
	if len(configured) == 0 {
		fmt.Printf("No models in %s.\n", localSettingsPath)
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MODEL\tNAME\tCONTEXT WINDOW\tAPI URL")
	for _, m := range configured {
		window := "-"
		if m.contextWindow > 0 {
			window = formatCount(m.contextWindow)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.id, m.name, window, m.apiURL)
	}
	return w.Flush()
}

// modelsInfo looks up the live OpenRouter catalog. A missing or unreadable
// settings.local.yaml still leaves the public catalog searchable, so this
// only warns, unlike modelsList where the local file is the entire answer.
func modelsInfo(query string) error {
	configured, err := readLocalModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	catalog, err := fetchOpenRouterModels(ctx, apiKeyFor(configured, query))
	if err != nil {
		return err
	}

	matches := matchOpenRouterModels(catalog, query)
	if len(matches) == 0 {
		return fmt.Errorf("no OpenRouter model matches %q%s", query, configuredHint(configured))
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })
	// Info blocks run several lines each, so a broad query stays readable at a
	// lower cap than a one-line-per-model listing would need.
	const shown = 5
	if len(matches) > shown {
		fmt.Printf("%d models match %q; showing the first %d — narrow the search term\n\n", len(matches), query, shown)
		matches = matches[:shown]
	}
	for i, m := range matches {
		if i > 0 {
			fmt.Println()
		}
		printModelInfo(m)
	}
	return nil
}

// modelsInfoLocal prints full OpenRouter details for every model configured
// in settings.local.yaml, in the order they appear there — the "info every
// model I actually use" shortcut for "models info <id>" run once per entry.
// Unlike modelsInfo, a missing or unreadable settings.local.yaml is fatal
// here: with no query, the local file is the entire input.
func modelsInfoLocal() error {
	configured, err := readLocalModels()
	if err != nil {
		return err
	}
	if len(configured) == 0 {
		fmt.Printf("No models in %s.\n", localSettingsPath)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	catalog, err := fetchOpenRouterModels(ctx, apiKeyFor(configured, ""))
	if err != nil {
		return err
	}
	byID := make(map[string]openRouterModel, len(catalog))
	for _, m := range catalog {
		byID[m.ID] = m
	}

	for i, cfg := range configured {
		if i > 0 {
			fmt.Println()
		}
		live, ok := byID[cfg.id]
		if !ok {
			fmt.Printf("%s — not found on OpenRouter\n", cfg.id)
			continue
		}
		printModelInfo(live)
	}
	return nil
}

// modelsCheck flags configuration drift: a context_window in settings.local.yaml
// that no longer matches what OpenRouter actually offers, or a model id
// OpenRouter no longer lists at all. Provider catalogs change without notice,
// so this is the only way that drift would otherwise surface — a live call
// failing with a confusing context-length error.
func modelsCheck() error {
	configured, err := readLocalModels()
	if err != nil {
		return err
	}
	if len(configured) == 0 {
		fmt.Printf("No models in %s.\n", localSettingsPath)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	catalog, err := fetchOpenRouterModels(ctx, apiKeyFor(configured, ""))
	if err != nil {
		return err
	}
	byID := make(map[string]openRouterModel, len(catalog))
	for _, m := range catalog {
		byID[m.ID] = m
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MODEL\tCONFIGURED\tACTUAL\tSTATUS")
	problems := 0
	for _, cfg := range configured {
		live, ok := byID[cfg.id]
		if !ok {
			fmt.Fprintf(w, "%s\t%s\t-\tnot found on OpenRouter\n", cfg.id, formatContext(cfg.contextWindow))
			problems++
			continue
		}
		actual := live.ContextLength
		if live.TopProvider.ContextLength > 0 {
			actual = live.TopProvider.ContextLength
		}
		status := "ok"
		if cfg.contextWindow != actual {
			status = "mismatch"
			problems++
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", cfg.id, formatContext(cfg.contextWindow), formatContext(actual), status)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if problems > 0 {
		return fmt.Errorf("%d of %d configured models have a context_window mismatch or are missing from OpenRouter", problems, len(configured))
	}
	fmt.Printf("%d models match OpenRouter's catalog.\n", len(configured))
	return nil
}

func formatContext(n int) string {
	if n <= 0 {
		return "-"
	}
	return formatCount(n)
}

// settingsModel is the slice of a settings.local.yaml model entry mk needs.
// The full shape is internal/config.ModelEntry, but mk depends only on the
// standard library, so this reads the file with a small parser tailored to the
// fixed "models: - key: value" shape the file actually has, rather than pulling
// in a YAML library or the config package.
//
// keep_alive and integration are absent because neither has a catalog column.
// keep_alive tunes how long a local daemon holds a model in memory for the
// client that called it, and integration names a gateway profile the local
// client trusts; both are properties of this machine rather than of the target.
type settingsModel struct {
	id            string
	name          string
	provider      string
	transport     string
	apiURL        string
	apiKey        string
	contextWindow int
	callTimeout   int
	maxTokens     int
	reasoning     string
	cacheMode     string
	cacheTTL      string
	pricing       settingsPricing
	vision        bool
}

// settingsPricing is the nested pricing block. Prices stay strings all the way
// to the add command: they are decimal rates, and mk has no reason to parse one
// it only forwards.
type settingsPricing struct {
	currency          string
	inputPerMTok      string
	cacheReadPerMTok  string
	cacheWritePerMTok string
	outputPerMTok     string
}

// isManaged reports whether an older entry already calls a BuildMax gateway
// rather than a provider. Such an entry names a catalog row, not an upstream,
// so there is nothing in it to put in a catalog. Current client modes no longer
// create these entries, but the reader still ignores one left in an older file.
func (m settingsModel) isManaged() bool { return m.transport == "buildmax" }

// readLocalModels reads settings.local.yaml from the repository root. main's
// dispatch already chdirs there before running any command, so the bare
// relative name is enough.
func readLocalModels() ([]settingsModel, error) {
	data, err := os.ReadFile(localSettingsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s not found; copy the template to get started: cp %s %s",
			localSettingsPath, localSettingsExample, localSettingsPath)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", localSettingsPath, err)
	}
	return parseSettingsModels(string(data)), nil
}

// parseSettingsModels reads the "models:" list. Each entry starts at a
// "  - key: value" line and continues through subsequent "    key: value"
// lines until the next "- " or a line back at column 0. That is the only
// shape this file's models block has ever had; a real YAML parser is not
// worth the dependency for five fields.
//
// cache_control and pricing are one level deeper, so a key is read against the
// block it sits under: a bare "mode:" means nothing on its own, and pricing
// and cache_control could otherwise claim each other's keys.
func parseSettingsModels(text string) []settingsModel {
	var models []settingsModel
	var current *settingsModel
	inModels := false
	block := ""
	blockIndent := 0

	flush := func() {
		if current != nil && current.id != "" {
			models = append(models, *current)
		}
		current = nil
		block = ""
	}

	for raw := range strings.SplitSeq(text, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))

		if indent == 0 {
			flush()
			inModels = trimmed == "models:"
			continue
		}
		if !inModels {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			flush()
			current = &settingsModel{}
			trimmed = strings.TrimPrefix(trimmed, "- ")
			indent += 2
		}
		if current == nil {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = unquote(strings.TrimSpace(value))
		if block != "" && indent <= blockIndent {
			block = ""
		}
		if block == "" && value == "" && (key == "cache_control" || key == "pricing") {
			block, blockIndent = key, indent
			continue
		}
		switch block {
		case "cache_control":
			switch key {
			case "mode":
				current.cacheMode = value
			case "ttl":
				current.cacheTTL = value
			}
			continue
		case "pricing":
			switch key {
			case "currency":
				current.pricing.currency = value
			case "input_per_mtok":
				current.pricing.inputPerMTok = value
			case "cache_read_per_mtok":
				current.pricing.cacheReadPerMTok = value
			case "cache_write_per_mtok":
				current.pricing.cacheWritePerMTok = value
			case "output_per_mtok":
				current.pricing.outputPerMTok = value
			}
			continue
		}
		switch key {
		case "model":
			current.id = value
		case "name":
			current.name = value
		case "api_url":
			current.apiURL = value
		case "api_key":
			current.apiKey = value
		case "context_window":
			current.contextWindow, _ = strconv.Atoi(value)
		case "call_timeout":
			current.callTimeout, _ = strconv.Atoi(value)
		case "max_tokens":
			current.maxTokens, _ = strconv.Atoi(value)
		case "provider":
			current.provider = value
		case "transport":
			current.transport = value
		case "reasoning":
			current.reasoning = value
		case "vision":
			current.vision = value == "true"
		}
	}
	flush()
	return models
}

// apiKeyFor picks the key to send with the catalog request: the configured
// model matching the query first (its price may be account-specific), then
// any configured key, since OpenRouter keys are interchangeable across
// models on the same account.
func apiKeyFor(configured []settingsModel, query string) string {
	lower := strings.ToLower(query)
	for _, m := range configured {
		if strings.Contains(strings.ToLower(m.id), lower) && m.apiKey != "" {
			return m.apiKey
		}
	}
	for _, m := range configured {
		if m.apiKey != "" {
			return m.apiKey
		}
	}
	return ""
}

func configuredHint(configured []settingsModel) string {
	if len(configured) == 0 {
		return ""
	}
	ids := make([]string, len(configured))
	for i, m := range configured {
		ids[i] = m.id
	}
	return fmt.Sprintf(" (%s configures: %s)", localSettingsPath, strings.Join(ids, ", "))
}

// openRouterPricing is OpenRouter's documented set of pricing keys. Each rate
// is a decimal-dollar string ("0.0000001"), not cents or a fixed-width
// number, so it is parsed on demand rather than at unmarshal time. A model
// with volume-tiered pricing (e.g. a discount past some prompt length) adds
// Overrides instead of changing these; a model without any override rides on
// the base rates above.
type openRouterPricing struct {
	Prompt            string      `json:"prompt"`
	Completion        string      `json:"completion"`
	Request           string      `json:"request"`
	Image             string      `json:"image"`
	WebSearch         string      `json:"web_search"`
	InternalReasoning string      `json:"internal_reasoning"`
	InputCacheRead    string      `json:"input_cache_read"`
	InputCacheWrite   string      `json:"input_cache_write"`
	Audio             string      `json:"audio"`
	Overrides         []priceTier `json:"overrides"`
}

// priceTier is one entry of a tiered/volume pricing schedule: the base rates
// change once a request's prompt crosses MinPromptTokens.
type priceTier struct {
	MinPromptTokens int    `json:"min_prompt_tokens"`
	Prompt          string `json:"prompt"`
	Completion      string `json:"completion"`
	InputCacheRead  string `json:"input_cache_read"`
	InputCacheWrite string `json:"input_cache_write"`
}

// openRouterModel is the subset of OpenRouter's /models response this
// command prints: identity, capabilities, and pricing.
type openRouterModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Created       int64  `json:"created"`
	ContextLength int    `json:"context_length"`
	Architecture  struct {
		Modality  string `json:"modality"`
		Tokenizer string `json:"tokenizer"`
	} `json:"architecture"`
	Pricing     openRouterPricing `json:"pricing"`
	TopProvider struct {
		ContextLength       int  `json:"context_length"`
		MaxCompletionTokens int  `json:"max_completion_tokens"`
		IsModerated         bool `json:"is_moderated"`
	} `json:"top_provider"`
	SupportedParameters []string `json:"supported_parameters"`
	KnowledgeCutoff     string   `json:"knowledge_cutoff"`
}

func fetchOpenRouterModels(ctx context.Context, apiKey string) ([]openRouterModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterModelsURL, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", openRouterModelsURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read OpenRouter response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenRouter returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		Data []openRouterModel `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse OpenRouter response: %w", err)
	}
	return parsed.Data, nil
}

// matchOpenRouterModels finds an exact id match first; a query that pins one
// model should never be widened by a substring collision. Failing that, it
// falls back to a case-insensitive substring over the id and display name.
func matchOpenRouterModels(catalog []openRouterModel, query string) []openRouterModel {
	for _, m := range catalog {
		if m.ID == query {
			return []openRouterModel{m}
		}
	}
	lower := strings.ToLower(query)
	var matches []openRouterModel
	for _, m := range catalog {
		if strings.Contains(strings.ToLower(m.ID), lower) || strings.Contains(strings.ToLower(m.Name), lower) {
			matches = append(matches, m)
		}
	}
	return matches
}

func printModelInfo(m openRouterModel) {
	fmt.Printf("%s — %s\n", m.ID, m.Name)
	if m.Created > 0 {
		fmt.Printf("  added to OpenRouter: %s\n", time.Unix(m.Created, 0).UTC().Format("2006-01-02"))
	}
	if m.KnowledgeCutoff != "" {
		fmt.Printf("  knowledge cutoff: %s\n", m.KnowledgeCutoff)
	}

	window := m.ContextLength
	if m.TopProvider.ContextLength > 0 {
		window = m.TopProvider.ContextLength
	}
	fmt.Printf("  context window: %s tokens", formatCount(window))
	if m.TopProvider.MaxCompletionTokens > 0 {
		fmt.Printf(" (max output: %s)", formatCount(m.TopProvider.MaxCompletionTokens))
	}
	fmt.Println()

	if m.Architecture.Modality != "" {
		fmt.Printf("  modality: %s\n", m.Architecture.Modality)
	}
	if m.Architecture.Tokenizer != "" {
		fmt.Printf("  tokenizer: %s\n", m.Architecture.Tokenizer)
	}
	if m.TopProvider.IsModerated {
		fmt.Println("  moderated: yes")
	}

	if rows := pricingRows(m.Pricing); len(rows) > 0 {
		fmt.Println("  pricing:")
		for _, row := range rows {
			fmt.Printf("    %s\n", row)
		}
	}

	if len(m.SupportedParameters) > 0 {
		fmt.Println("  supported parameters:")
		fmt.Print(wrapText(strings.Join(m.SupportedParameters, ", "), 76, "    "))
	}

	if m.Description != "" {
		fmt.Println("  description:")
		fmt.Print(wrapText(m.Description, 76, "    "))
	}
}

// pricingRows renders the base rates, in the order a pricing page would
// (token rates, then flat per-unit ones), followed by any volume-tiered
// override schedule.
func pricingRows(p openRouterPricing) []string {
	var rows []string
	add := func(label, raw string, perToken, always bool) {
		if row := formatPriceRow(label, raw, perToken, always); row != "" {
			rows = append(rows, row)
		}
	}
	add("input", p.Prompt, true, true)
	add("output", p.Completion, true, true)
	add("cached input (read)", p.InputCacheRead, true, false)
	add("cached input (write)", p.InputCacheWrite, true, false)
	add("internal reasoning", p.InternalReasoning, true, false)
	add("audio input", p.Audio, true, false)
	add("per request", p.Request, false, false)
	add("per image", p.Image, false, false)
	add("per web search", p.WebSearch, false, false)
	for _, tier := range p.Overrides {
		if row := formatPriceTier(tier); row != "" {
			rows = append(rows, row)
		}
	}
	return rows
}

// formatPriceRow renders one base rate. perToken=false, always=true is never
// used: an "each" price is never shown as "free" the way a $0 token rate is,
// since the field is simply absent from models that don't charge per-unit at
// all.
func formatPriceRow(label, raw string, perToken, always bool) string {
	price, err := strconv.ParseFloat(raw, 64)
	if err != nil || price < 0 {
		return ""
	}
	if price == 0 {
		if !always {
			return ""
		}
		return label + ": free"
	}
	if perToken {
		return fmt.Sprintf("%s: $%.4f / 1M tokens", label, price*1_000_000)
	}
	return fmt.Sprintf("%s: $%.4f each", label, price)
}

// formatPriceTier renders one entry of a volume-tiered pricing schedule.
func formatPriceTier(t priceTier) string {
	var parts []string
	if price, err := strconv.ParseFloat(t.Prompt, 64); err == nil && price > 0 {
		parts = append(parts, fmt.Sprintf("input $%.4f/1M", price*1_000_000))
	}
	if price, err := strconv.ParseFloat(t.Completion, 64); err == nil && price > 0 {
		parts = append(parts, fmt.Sprintf("output $%.4f/1M", price*1_000_000))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("above %s prompt tokens: %s", formatCount(t.MinPromptTokens), strings.Join(parts, ", "))
}

// wrapText word-wraps at width, indenting every line, and always ends with a
// trailing newline so callers can Print it directly.
func wrapText(text string, width int, indent string) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(indent)
	lineLen := 0
	for i, word := range words {
		if i > 0 {
			if lineLen+1+len(word) > width {
				b.WriteString("\n")
				b.WriteString(indent)
				lineLen = 0
			} else {
				b.WriteString(" ")
				lineLen++
			}
		}
		b.WriteString(word)
		lineLen += len(word)
	}
	b.WriteString("\n")
	return b.String()
}

func formatCount(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(digit)
	}
	return b.String()
}
