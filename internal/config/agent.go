package config

// Parallel tool execution bounds. The ceiling protects file descriptors,
// memory for large reads, and remote rate limits for WebFetch and MCP; real
// batches from a model are two to five calls, so the default captures nearly
// all of the available win while keeping the failure modes boring.
const (
	DefaultMaxParallelTools = 4
	MinMaxParallelTools     = 1
	MaxMaxParallelTools     = 16
)

// Agent loop iteration bounds. The cap exists so a run that stopped making
// progress ends instead of spending until the credential does, and 200 covers
// ordinary interactive work several times over. The ceiling is what a long
// autonomous task may raise it to: an agent still calling tools after five
// thousand iterations is looping, not working, and no honest task needs the
// difference between that and unbounded.
const (
	DefaultMaxIterations = 200
	MinMaxIterations     = 1
	MaxMaxIterations     = 5000
)

// AgentConfig is the "agent" block of settings.yaml.
type AgentConfig struct {
	// MaxParallelTools bounds how many tool calls from one assistant message
	// may run at once. 1 disables parallel execution; 0 means unset.
	MaxParallelTools int `mapstructure:"max_parallel_tools" json:"max_parallel_tools,omitempty" yaml:"max_parallel_tools,omitempty"`
	// MaxIterations bounds how many times the agent loop may call the model in
	// one run. 0 means unset.
	MaxIterations int `mapstructure:"max_iterations" json:"max_iterations,omitempty" yaml:"max_iterations,omitempty"`
	// TurnDigest controls the after-the-turn side call. See TurnDigestConfig.
	TurnDigest TurnDigestConfig `mapstructure:"turn_digest" json:"turn_digest" yaml:"turn_digest"`
}

// TurnDigestConfig is the "agent.turn_digest" block: one extra model call at
// the end of a turn that writes a recap of what the turn did and predicts the
// answer the user is about to type. Neither ever enters the conversation.
//
// Both parts default on, so the fields are pointers: a plain bool cannot tell
// "the file said false" from "the file said nothing", and the answer to a
// billed behavior must not depend on that distinction.
type TurnDigestConfig struct {
	// Recap prints a dim summary of the turn under the reply.
	Recap *bool `mapstructure:"recap" json:"recap,omitempty" yaml:"recap,omitempty"`
	// Suggest offers the predicted answer as ghost text in the input box.
	Suggest *bool `mapstructure:"suggest" json:"suggest,omitempty" yaml:"suggest,omitempty"`
}

// TurnDigestRecap and TurnDigestSuggest report whether each part is on.
func TurnDigestRecap(cfg TurnDigestConfig) bool   { return cfg.Recap == nil || *cfg.Recap }
func TurnDigestSuggest(cfg TurnDigestConfig) bool { return cfg.Suggest == nil || *cfg.Suggest }

// ResolveMaxParallelTools clamps the configured value into the supported range.
// Out-of-range is clamped rather than rejected: a number that is merely too
// large should not stop the agent starting.
func ResolveMaxParallelTools(cfg AgentConfig) int {
	switch {
	case cfg.MaxParallelTools == 0:
		return DefaultMaxParallelTools
	case cfg.MaxParallelTools < MinMaxParallelTools:
		return MinMaxParallelTools
	case cfg.MaxParallelTools > MaxMaxParallelTools:
		return MaxMaxParallelTools
	default:
		return cfg.MaxParallelTools
	}
}

// ResolveMaxIterations settles the loop cap for one run. A per-run override
// outranks settings.yaml, which outranks the default; zero at either level
// means unset rather than "no iterations", because a cap of nothing is a value
// nobody can mean.
//
// Out-of-range is clamped rather than rejected, for the reason
// ResolveMaxParallelTools gives.
func ResolveMaxIterations(cfg AgentConfig, override int) int {
	want := override
	if want == 0 {
		want = cfg.MaxIterations
	}
	switch {
	case want == 0:
		return DefaultMaxIterations
	case want < MinMaxIterations:
		return MinMaxIterations
	case want > MaxMaxIterations:
		return MaxMaxIterations
	default:
		return want
	}
}
