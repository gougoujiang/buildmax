package llmgateway_test

import (
	"slices"
	"testing"

	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

func TestCapabilitySetHas(t *testing.T) {
	set := llmgateway.NewCapabilitySet(llmgateway.CapabilityTextChat, llmgateway.CapabilityToolCalls)
	if !set.Has(llmgateway.CapabilityTextChat) {
		t.Error("declared capability reported as absent")
	}
	if set.Has(llmgateway.CapabilityStreamingText) {
		t.Error("undeclared capability reported as present")
	}
	if llmgateway.CapabilitySet(nil).Has(llmgateway.CapabilityTextChat) {
		t.Error("the zero set must declare nothing")
	}
}

func TestNewCapabilitySetIgnoresEmptyAndDuplicate(t *testing.T) {
	set := llmgateway.NewCapabilitySet(
		llmgateway.CapabilityTextChat,
		llmgateway.CapabilityTextChat,
		"",
	)
	if len(set) != 1 {
		t.Fatalf("want 1 capability, got %d: %v", len(set), set.List())
	}
}

func TestCapabilitySetMissing(t *testing.T) {
	set := llmgateway.NewCapabilitySet(llmgateway.CapabilityTextChat)

	tests := []struct {
		name     string
		required []llmgateway.Capability
		want     []llmgateway.Capability
	}{
		{
			name:     "no requirements",
			required: nil,
			want:     nil,
		},
		{
			name:     "all met",
			required: []llmgateway.Capability{llmgateway.CapabilityTextChat},
			want:     nil,
		},
		{
			name:     "reports only what is absent",
			required: []llmgateway.Capability{llmgateway.CapabilityTextChat, llmgateway.CapabilityToolCalls},
			want:     []llmgateway.Capability{llmgateway.CapabilityToolCalls},
		},
		{
			name: "keeps requested order and deduplicates",
			required: []llmgateway.Capability{
				llmgateway.CapabilityStreamingText,
				llmgateway.CapabilityToolCalls,
				llmgateway.CapabilityStreamingText,
			},
			want: []llmgateway.Capability{llmgateway.CapabilityStreamingText, llmgateway.CapabilityToolCalls},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := set.Missing(tc.required)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Missing(%v) = %v, want %v", tc.required, got, tc.want)
			}
		})
	}
}

func TestCapabilitySetListIsStable(t *testing.T) {
	set := llmgateway.NewCapabilitySet(BaselineShuffled()...)
	want := []llmgateway.Capability{
		llmgateway.CapabilityStreamingText,
		llmgateway.CapabilityTextChat,
		llmgateway.CapabilityToolCalls,
		llmgateway.CapabilityUsageReporting,
	}
	for range 5 {
		if got := set.List(); !slices.Equal(got, want) {
			t.Fatalf("List() = %v, want %v", got, want)
		}
	}
	if llmgateway.CapabilitySet(nil).List() != nil {
		t.Error("the zero set must list nothing")
	}
}

// BaselineShuffled returns the baseline capabilities in a non-sorted order, so
// List cannot pass by accident.
func BaselineShuffled() []llmgateway.Capability {
	return []llmgateway.Capability{
		llmgateway.CapabilityUsageReporting,
		llmgateway.CapabilityTextChat,
		llmgateway.CapabilityStreamingText,
		llmgateway.CapabilityToolCalls,
	}
}

func TestBaselineCapabilitiesMatchesCoreContract(t *testing.T) {
	set := llmgateway.NewCapabilitySet(llmgateway.BaselineCapabilities()...)
	// The core llm.LLMClient contract is blocking chat, streaming chat, tool
	// definitions and tool calls, and usage on both paths.
	required := []llmgateway.Capability{
		llmgateway.CapabilityTextChat,
		llmgateway.CapabilityToolCalls,
		llmgateway.CapabilityStreamingText,
		llmgateway.CapabilityUsageReporting,
	}
	if missing := set.Missing(required); len(missing) > 0 {
		t.Errorf("baseline is missing %v", missing)
	}
}
