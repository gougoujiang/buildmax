package config

import "testing"

// Prices are written as decimal strings because that is how providers publish
// them, and a value that can be compared against a price page without
// arithmetic is one a mistake shows up in.
func TestResolvePricing(t *testing.T) {
	got, err := ResolvePricing(&ModelPricing{
		Currency:          "USD",
		InputPerMTok:      "3.00",
		CacheReadPerMTok:  "0.30",
		CacheWritePerMTok: "3.75",
		OutputPerMTok:     "15.00",
	})
	if err != nil {
		t.Fatalf("ResolvePricing: %v", err)
	}
	if got.Currency != "USD" || got.InputPerMTok != 3_000_000_000 ||
		got.CacheReadPerMTok != 300_000_000 || got.CacheWritePerMTok != 3_750_000_000 ||
		got.OutputPerMTok != 15_000_000_000 {
		t.Errorf("resolved %+v", got)
	}
	if !got.Configured() {
		t.Error("a complete price list should be usable")
	}
}

// Pricing is optional. A model without it reports its cost as unavailable
// rather than as zero, so an absent block is not an error.
func TestResolvePricingAcceptsNothing(t *testing.T) {
	got, err := ResolvePricing(nil)
	if err != nil {
		t.Fatalf("ResolvePricing(nil): %v", err)
	}
	if got.Configured() {
		t.Error("an absent price list should not be usable")
	}
}

// Half a price list produces a number that looks authoritative and is not, so
// it is refused where the file can be named rather than silently completed with
// zeroes.
func TestResolvePricingRefusesAHalfList(t *testing.T) {
	tests := map[string]*ModelPricing{
		"rates with no currency":  {InputPerMTok: "3.00"},
		"a currency with no rate": {Currency: "USD"},
		"a malformed rate":        {Currency: "USD", InputPerMTok: "three dollars"},
		"a negative rate":         {Currency: "USD", InputPerMTok: "-1"},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolvePricing(in); err == nil {
				t.Error("expected a refusal")
			}
		})
	}
}

// A provider that does not charge for cache reads is a real price list, not an
// incomplete one.
func TestResolvePricingAcceptsAZeroRate(t *testing.T) {
	got, err := ResolvePricing(&ModelPricing{
		Currency: "USD", InputPerMTok: "3.00", CacheReadPerMTok: "0", OutputPerMTok: "15.00",
	})
	if err != nil {
		t.Fatalf("ResolvePricing: %v", err)
	}
	if !got.Configured() || got.CacheReadPerMTok != 0 {
		t.Errorf("resolved %+v", got)
	}
}
