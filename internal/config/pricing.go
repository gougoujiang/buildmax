package config

import (
	"fmt"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// ModelPricing is what one model charges, as it is written in a settings file
// (snake_case on disk).
//
// The rates are decimal strings quoted per million tokens, which is how every
// provider publishes them: a configured value can be checked against a price
// page without arithmetic, and a YAML float would have rounded it before
// anything here saw it. They are parsed into the fixed-point form in
// core/llm.Pricing, because a run of a few hundred calls accumulates float
// error into a figure someone will compare against an invoice.
//
// The four rates are separate because prompt caching prices them differently: a
// cache read is cheaper than fresh input and a cache write is dearer, which is
// the whole reason caching is a decision rather than a free win.
type ModelPricing struct {
	// Currency is the ISO 4217 code the rates are quoted in. Required: rates
	// with no currency price nothing, because nothing can be added to them.
	Currency string `mapstructure:"currency"`
	// InputPerMTok is fresh prompt input, excluding anything cached.
	InputPerMTok string `mapstructure:"input_per_mtok"`
	// CacheReadPerMTok is prompt served from the provider's cache, and
	// CacheWritePerMTok is prompt written into it. Left empty they are zero,
	// which is a real price on a provider that does not charge for one.
	CacheReadPerMTok  string `mapstructure:"cache_read_per_mtok"`
	CacheWritePerMTok string `mapstructure:"cache_write_per_mtok"`
	// OutputPerMTok is generated tokens.
	OutputPerMTok string `mapstructure:"output_per_mtok"`
}

// ResolvePricing parses a configured price list.
//
// A nil entry is not an error: pricing is optional, and a model without it
// reports its cost as unavailable rather than as zero. A malformed one is an
// error, because someone wrote a price and meant it.
func ResolvePricing(in *ModelPricing) (cllm.Pricing, error) {
	if in == nil {
		return cllm.Pricing{}, nil
	}
	out := cllm.Pricing{Currency: in.Currency}
	for _, field := range []struct {
		name  string
		value string
		into  *int64
	}{
		{"input_per_mtok", in.InputPerMTok, &out.InputPerMTok},
		{"cache_read_per_mtok", in.CacheReadPerMTok, &out.CacheReadPerMTok},
		{"cache_write_per_mtok", in.CacheWritePerMTok, &out.CacheWritePerMTok},
		{"output_per_mtok", in.OutputPerMTok, &out.OutputPerMTok},
	} {
		parsed, err := cllm.ParseRate(field.value)
		if err != nil {
			return cllm.Pricing{}, fmt.Errorf("pricing.%s: %w", field.name, err)
		}
		*field.into = parsed
	}
	// Half a price list produces a number that looks authoritative and is not.
	// Refusing here names the file; refusing later would name nothing.
	anyRate := out.InputPerMTok != 0 || out.CacheReadPerMTok != 0 ||
		out.CacheWritePerMTok != 0 || out.OutputPerMTok != 0
	switch {
	case out.Currency == "" && anyRate:
		return cllm.Pricing{}, fmt.Errorf("pricing.currency is required when any rate is set")
	case out.Currency != "" && !anyRate:
		return cllm.Pricing{}, fmt.Errorf("pricing.currency %q is set but no rate is", out.Currency)
	}
	return out, nil
}
