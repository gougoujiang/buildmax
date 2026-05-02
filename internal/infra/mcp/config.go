package mcp

// ConfigRoot is the root of mcp.json (top-level key mcpServers).
type ConfigRoot struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// ServerConfig is one server entry in mcpServers.
type ServerConfig struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
}
