package llmgateway

import "slices"

// Capability is a model behavior a caller can require and an operator must
// declare. Capabilities are declared per catalog target rather than inferred
// from a provider name, so a request needing an unsupported capability fails
// before an upstream call is made.
type Capability string

const (
	// CapabilityTextChat is chat completion over messages.
	CapabilityTextChat Capability = "text_chat"
	// CapabilityToolCalls is tool definition input and tool call output.
	CapabilityToolCalls Capability = "tool_calls"
	// CapabilityStreamingText is incremental content delivery during a call.
	CapabilityStreamingText Capability = "streaming_text"
	// CapabilityUsageReporting is token usage returned with the result.
	CapabilityUsageReporting Capability = "usage_reporting"
	// CapabilityImageInput is image content accepted in a request. It is not in
	// BaselineCapabilities: most models do not have it, and a request carrying
	// an image is rejected rather than ignored by one that does not.
	CapabilityImageInput Capability = "image_input"
)

// BaselineCapabilities is the capability set matching the current
// core/llm.LLMClient contract. An operator configuring an OpenAI-compatible
// target normally declares exactly this set.
func BaselineCapabilities() []Capability {
	return []Capability{
		CapabilityTextChat,
		CapabilityToolCalls,
		CapabilityStreamingText,
		CapabilityUsageReporting,
	}
}

// CapabilitySet is the set of capabilities one target declares. The zero value
// declares nothing: a target must state what it supports.
type CapabilitySet map[Capability]struct{}

// NewCapabilitySet builds a set, ignoring duplicates and empty values.
func NewCapabilitySet(capabilities ...Capability) CapabilitySet {
	set := make(CapabilitySet, len(capabilities))
	for _, capability := range capabilities {
		if capability == "" {
			continue
		}
		set[capability] = struct{}{}
	}
	return set
}

// Has reports whether the set declares the capability.
func (s CapabilitySet) Has(capability Capability) bool {
	_, ok := s[capability]
	return ok
}

// Missing returns the required capabilities the set does not declare, keeping
// the order they were requested in. It returns nil when every requirement is
// met, so callers can test the result directly.
func (s CapabilitySet) Missing(required []Capability) []Capability {
	var missing []Capability
	seen := make(map[Capability]struct{}, len(required))
	for _, capability := range required {
		if capability == "" {
			continue
		}
		if _, duplicate := seen[capability]; duplicate {
			continue
		}
		seen[capability] = struct{}{}
		if !s.Has(capability) {
			missing = append(missing, capability)
		}
	}
	return missing
}

// List returns the declared capabilities in a stable order, so listings and
// test assertions do not depend on map iteration.
func (s CapabilitySet) List() []Capability {
	if len(s) == 0 {
		return nil
	}
	out := make([]Capability, 0, len(s))
	for capability := range s {
		out = append(out, capability)
	}
	slices.Sort(out)
	return out
}
