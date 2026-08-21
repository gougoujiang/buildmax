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

// AgentConfig is the "agent" block of settings.yaml.
type AgentConfig struct {
	// MaxParallelTools bounds how many tool calls from one assistant message
	// may run at once. 1 disables parallel execution; 0 means unset.
	MaxParallelTools int `mapstructure:"max_parallel_tools" json:"max_parallel_tools,omitempty" yaml:"max_parallel_tools,omitempty"`
}

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
